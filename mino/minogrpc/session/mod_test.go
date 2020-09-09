package session

import (
	"context"
	"io"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc/ptypes"
	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestSession_Listen(t *testing.T) {
	sess := NewSession(
		nil,
		&fakeStream{num: 2},
		fake.NewAddress(999),
		fakeTable{err: xerrors.New("oops")},
		nil,
		fakePktFac{},
		fake.NewContext(),
		fakeConnMgr{},
	).(*session)

	sess.logger = zerolog.Nop()
	sess.relays[fake.NewAddress(100)] = newRelay(nil, nil, sess.context)

	sess.Listen()
	_, more := <-sess.errs
	require.False(t, more)

	sess.gateway = newRelay(&fakeStream{num: 1}, fakePktFac{dest: fake.NewAddress(0)}, sess.context)
	sess.errs = make(chan error)
	sess.Listen()
	_, more = <-sess.errs
	require.False(t, more)

	stream := &fakeStream{err: status.Error(codes.Canceled, "")}
	sess.gateway = newRelay(stream, sess.pktFac, sess.context)
	sess.errs = make(chan error)
	sess.Listen()
	_, more = <-sess.errs
	require.False(t, more)

	stream = &fakeStream{err: xerrors.New("oops")}
	sess.gateway = newRelay(stream, sess.pktFac, sess.context)
	sess.errs = make(chan error, 1)
	sess.Listen()
	err, more := <-sess.errs
	require.True(t, more)
	require.EqualError(t, err, "failed to receive: oops")
}

func TestSession_Send(t *testing.T) {
	stream := &fakeStream{calls: &fake.Call{}}
	sess := &session{
		gateway: newRelay(stream, fakePktFac{}, fake.NewContext()),
		table:   fakeTable{},
		context: fake.NewContext(),
		queue:   newNonBlockingQueue(),
		relays:  make(map[mino.Address]Relay),
	}

	errs := sess.Send(fake.Message{}, fake.NewAddress(0))
	require.NoError(t, <-errs)
	require.Equal(t, 1, stream.calls.Len())

	sess.table = fakeTable{route: fake.NewAddress(0)}
	sess.relays[fake.NewAddress(0)] = newRelay(&fakeStream{}, nil, sess.context)
	errs = sess.Send(fake.Message{}, fake.NewAddress(1))
	require.NoError(t, <-errs)

	errs = sess.Send(fake.NewBadPublicKey())
	require.EqualError(t, <-errs, "failed to serialize msg: fake error")

	sess.table = fakeTable{err: xerrors.New("oops")}
	errs = sess.Send(fake.Message{})
	require.EqualError(t, <-errs, "packet: routing table: oops")
}

func TestSession_SetupRelay(t *testing.T) {
	sess := &session{
		connMgr: fakeConnMgr{},
		table:   fakeTable{},
		context: fake.NewContext(),
		relays:  make(map[mino.Address]Relay),
		queue:   newNonBlockingQueue(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	relay, err := sess.setupRelay(ctx, fake.NewAddress(0))
	require.NoError(t, err)
	require.NotNil(t, relay)
	sess.Wait()

	sess.connMgr = fakeConnMgr{err: xerrors.New("oops")}
	_, err = sess.setupRelay(ctx, fake.NewAddress(1))
	require.EqualError(t, err, "failed to dial: oops")

	sess.connMgr = fakeConnMgr{errConn: xerrors.New("oops")}
	_, err = sess.setupRelay(ctx, fake.NewAddress(1))
	require.EqualError(t, err, "client: oops")

	sess.connMgr = fakeConnMgr{errStream: xerrors.New("oops")}
	_, err = sess.setupRelay(ctx, fake.NewAddress(1))
	require.EqualError(t, err, "failed to send handshake: oops")

	sess.connMgr = fakeConnMgr{}
	sess.table = fakeTable{err: xerrors.New("oops")}
	_, err = sess.setupRelay(ctx, fake.NewAddress(1))
	require.EqualError(t, err, "failed to serialize handshake: oops")

	sess.table = fakeTable{}
	sess.connMgr = fakeConnMgr{errRecv: status.Error(codes.Canceled, "")}
	_, err = sess.setupRelay(ctx, fake.NewAddress(1))
	require.NoError(t, err)
	sess.Wait()

	sess.connMgr = fakeConnMgr{errRecv: xerrors.New("oops")}
	_, err = sess.setupRelay(ctx, fake.NewAddress(2))
	require.NoError(t, err)
	sess.Wait()

	sess.connMgr = fakeConnMgr{msg: &ptypes.Packet{}}
	sess.pktFac = fakePktFac{dest: fake.NewAddress(500)}
	sess.table = fakeTable{route: fake.NewAddress(500)}
	sess.relays[fake.NewAddress(500)] = newRelay(&fakeStream{err: xerrors.New("oops")}, sess.pktFac, sess.context)
	_, err = sess.setupRelay(ctx, fake.NewAddress(3))
	require.NoError(t, err)
	sess.Wait()
}

func TestSession_Recv(t *testing.T) {
	sess := &session{
		queue:  newNonBlockingQueue(),
		msgFac: fake.MessageFactory{},
		errs:   make(chan error, 1),
	}

	sess.queue.Push(fakePkt{})
	sess.queue.Push(fakePkt{})

	ctx, cancel := context.WithCancel(context.Background())

	from, msg, err := sess.Recv(ctx)
	require.NoError(t, err)
	require.Equal(t, fake.NewAddress(700), from)
	require.Equal(t, fake.Message{}, msg)

	sess.msgFac = fake.NewBadMessageFactory()
	_, _, err = sess.Recv(ctx)
	require.EqualError(t, err, "message: fake error")

	sess.errs <- xerrors.New("oops")
	_, _, err = sess.Recv(ctx)
	require.EqualError(t, err, "stream closed unexpectedly: oops")

	sess.errs <- nil
	_, _, err = sess.Recv(ctx)
	require.Equal(t, io.EOF, err)

	cancel()
	_, _, err = sess.Recv(ctx)
	require.Equal(t, context.Canceled, err)
}

func TestRelay_Recv(t *testing.T) {
	r := newRelay(&fakeStream{num: 1}, fakePktFac{}, fake.NewContext())

	packet, err := r.Recv()
	require.NoError(t, err)
	require.Equal(t, fakePkt{}, packet)

	r = newRelay(&fakeStream{num: 1}, fakePktFac{err: xerrors.New("oops")}, fake.NewContext())
	_, err = r.Recv()
	require.EqualError(t, err, "packet: oops")
}

func TestRelay_Send(t *testing.T) {
	r := newRelay(&fakeStream{}, fakePktFac{}, fake.NewContext())

	err := r.Send(fakePkt{})
	require.NoError(t, err)

	err = r.Send(fakePkt{err: xerrors.New("oops")})
	require.EqualError(t, err, "failed to serialize: oops")

	r.stream = &fakeStream{err: xerrors.New("oops")}
	err = r.Send(fakePkt{})
	require.EqualError(t, err, "stream failed: oops")
}

func TestRelay_Close(t *testing.T) {
	r := newRelay(&fakeStream{}, fakePktFac{}, fake.NewContext())

	err := r.Close()
	require.NoError(t, err)

	r.stream = &fakeStream{err: xerrors.New("oops")}
	err = r.Close()
	require.EqualError(t, err, "failed to close stream: oops")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeStream struct {
	ptypes.Overlay_StreamClient

	num   int
	calls *fake.Call
	err   error
}

func (s *fakeStream) Context() context.Context {
	return context.Background()
}

func (s *fakeStream) Recv() (*ptypes.Packet, error) {
	s.calls.Add("Recv")
	if s.num > 0 {
		s.num--

		return &ptypes.Packet{}, nil
	}

	if s.err != nil {
		return nil, s.err
	}

	return nil, io.EOF
}

func (s *fakeStream) Send(p *ptypes.Packet) error {
	s.calls.Add("Send", p)
	return s.err
}

func (s *fakeStream) CloseSend() error {
	return s.err
}

type fakePktFac struct {
	router.PacketFactory
	dest mino.Address
	err  error
}

func (fac fakePktFac) PacketOf(serde.Context, []byte) (router.Packet, error) {
	return fakePkt{dest: fac.dest}, fac.err
}

type fakePkt struct {
	router.Packet
	dest mino.Address
	err  error
}

func (p fakePkt) GetSource() mino.Address {
	return fake.NewAddress(700)
}

func (p fakePkt) GetDestination() []mino.Address {
	if p.dest == nil {
		return nil
	}

	return []mino.Address{p.dest}
}

func (p fakePkt) GetMessage() []byte {
	return []byte(`{}`)
}

func (p fakePkt) Slice(mino.Address) router.Packet {
	return p
}

func (p fakePkt) Serialize(serde.Context) ([]byte, error) {
	return []byte(`{}`), p.err
}

type fakeTable struct {
	router.RoutingTable

	route mino.Address
	err   error
}

func (t fakeTable) Make(mino.Address, []mino.Address, []byte) router.Packet {
	return fakePkt{dest: fake.NewAddress(0)}
}

func (t fakeTable) Prelude(mino.Address) router.Handshake {
	return fakeHandshake{err: t.err}
}

func (t fakeTable) Forward(router.Packet) (router.Routes, error) {
	routes := router.Routes{t.route: fakePkt{}}
	return routes, t.err
}

type fakeHandshake struct {
	router.Handshake

	err error
}

func (h fakeHandshake) Serialize(serde.Context) ([]byte, error) {
	return []byte(`{}`), h.err
}

type fakeConnMgr struct {
	ConnectionManager

	msg       *ptypes.Packet
	err       error
	errConn   error
	errStream error
	errRecv   error
}

func (mgr fakeConnMgr) Acquire(mino.Address) (grpc.ClientConnInterface, error) {
	conn := fakeConnection{
		msg:       mgr.msg,
		err:       mgr.errConn,
		errStream: mgr.errStream,
		errRecv:   mgr.errRecv,
	}

	return conn, mgr.err
}

func (mgr fakeConnMgr) Release(mino.Address) {}

type fakeConnection struct {
	grpc.ClientConnInterface
	msg       *ptypes.Packet
	err       error
	errStream error
	errRecv   error
}

func (conn fakeConnection) NewStream(ctx context.Context, desc *grpc.StreamDesc,
	m string, opts ...grpc.CallOption) (grpc.ClientStream, error) {

	ch := make(chan *ptypes.Packet, 1)

	if conn.msg != nil {
		ch <- conn.msg
	}

	go func() {
		<-ctx.Done()
		close(ch)
	}()

	stream := &fakeClientStream{ch: ch, err: conn.errStream, errRecv: conn.errRecv}

	return stream, conn.err
}

type fakeClientStream struct {
	grpc.ClientStream
	ch      chan *ptypes.Packet
	err     error
	errRecv error
}

func (str *fakeClientStream) SendMsg(m interface{}) error {
	return str.err
}

func (str *fakeClientStream) RecvMsg(m interface{}) error {
	if str.errRecv != nil {
		return str.errRecv
	}

	msg, more := <-str.ch
	if !more {
		return io.EOF
	}

	*(m.(*ptypes.Packet)) = *msg
	return nil
}
