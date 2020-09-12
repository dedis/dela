package session

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/internal/traffic"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc/ptypes"
	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestSession_New(t *testing.T) {
	curr := os.Getenv(traffic.EnvVariable)
	defer os.Setenv(traffic.EnvVariable, curr)

	os.Setenv(traffic.EnvVariable, "log")
	sess := NewSession(nil, nil, fake.NewAddress(999), nil, nil, nil, fake.NewContext(), nil)
	require.NotNil(t, sess.(*session).traffic)

	os.Setenv(traffic.EnvVariable, "print")
	sess = NewSession(nil, nil, fake.NewAddress(999), nil, nil, nil, fake.NewContext(), nil)
	require.NotNil(t, sess.(*session).traffic)

	os.Unsetenv(traffic.EnvVariable)
	sess = NewSession(nil, nil, fake.NewAddress(999), nil, nil, nil, fake.NewContext(), nil)
	require.Nil(t, sess.(*session).traffic)
}

func TestSession_Listen(t *testing.T) {
	sess := &session{
		gateway: &streamRelay{},
		errs:    make(chan error),
	}

	sess.Listen(&fakeStream{})
	select {
	case <-sess.errs:
	default:
		t.Fatal("expect channel to be closed")
	}

	sess.errs = make(chan error, 1)
	sess.Listen(&fakeStream{err: xerrors.New("oops")})
	select {
	case err := <-sess.errs:
		require.EqualError(t, err, "stream closed unexpectedly: oops")
	default:
		t.Fatal("expect an error")
	}
}

func TestSession_RecvPacket(t *testing.T) {
	sess := &session{
		pktFac:  fakePktFac{},
		gateway: &streamRelay{stream: &fakeStream{}},
		queue:   newNonBlockingQueue(),
		table:   fakeTable{err: xerrors.New("bad route")},
	}

	ack, err := sess.RecvPacket(fake.NewAddress(0), &ptypes.Packet{})
	require.NoError(t, err)
	require.NotEmpty(t, ack.Errors)
	require.Equal(t, "no route to fake.Address[400]: bad route", ack.GetErrors()[0])

	sess.pktFac = fakePktFac{err: xerrors.New("oops")}
	_, err = sess.RecvPacket(fake.NewAddress(0), &ptypes.Packet{})
	require.EqualError(t, err, "packet malformed: oops")
}

func TestSession_Send(t *testing.T) {
	stream := &fakeStream{calls: &fake.Call{}}
	sess := &session{
		gateway: NewStreamRelay(nil, stream, fake.NewContext()),
		table:   fakeTable{},
		context: fake.NewContext(),
		queue:   newNonBlockingQueue(),
		relays:  make(map[mino.Address]Relay),
	}

	errs := sess.Send(fake.Message{}, fake.NewAddress(0))
	require.NoError(t, <-errs)
	require.Equal(t, 1, stream.calls.Len())

	sess.table = fakeTable{route: fake.NewAddress(0)}
	sess.relays[fake.NewAddress(0)] = NewStreamRelay(nil, &fakeStream{}, sess.context)
	errs = sess.Send(fake.Message{}, fake.NewAddress(1))
	require.NoError(t, <-errs)

	errs = sess.Send(fake.NewBadPublicKey())
	require.EqualError(t, <-errs, "failed to serialize msg: fake error")
	require.NoError(t, <-errs)

	sess.table = fakeTable{err: xerrors.New("oops")}
	errs = sess.Send(fake.Message{})
	require.EqualError(t, <-errs, "no route to fake.Address[400]: oops")
	require.NoError(t, <-errs)

	// Test when an error occurred when setting up a relay, which moves to the
	// failure handler that will then fail on the routing table.
	sess.table = fakeTable{errFail: xerrors.New("oops"), route: fake.NewAddress(5)}
	sess.connMgr = fakeConnMgr{err: xerrors.New("blabla")}
	errs = sess.Send(fake.Message{})
	require.EqualError(t, <-errs, "no route to fake.Address[5]: oops")
	require.NoError(t, <-errs)

	// Test when an error occurred when forwarding a message to a relay, which
	// moves to the failure handler that will fail on the routing table.
	sess.relays[fake.NewAddress(6)] = NewStreamRelay(
		fake.NewAddress(800),
		&fakeStream{err: xerrors.New("oops")},
		sess.context)

	sess.table = fakeTable{errFail: xerrors.New("unavailable"), route: fake.NewAddress(6)}
	sess.gateway = NewStreamRelay(nil, &fakeStream{err: xerrors.New("oops")}, sess.context)
	errs = sess.Send(fake.Message{})
	require.EqualError(t, <-errs, "no route to fake.Address[800]: unavailable")
	require.NoError(t, <-errs)

	// Test when the parent stream has closed.
	sess.me = fake.NewAddress(123)
	sess.table = fakeTable{}
	sess.gateway = NewStreamRelay(nil, &fakeStream{err: xerrors.New("oops")}, sess.context)
	errs = sess.Send(fake.Message{})
	require.EqualError(t, <-errs, "session fake.Address[123] is closing: OK")

	// Test when a packet is sent but some addresses are not reachable.
	sess.gateway = &unicastRelay{
		stream: &fakeStream{},
		conn:   fakeConnection{ack: &ptypes.Ack{Errors: []string{"bad route"}}},
	}
	errs = sess.Send(fake.Message{})
	require.EqualError(t, <-errs, "bad route")
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

	sess.connMgr = fakeConnMgr{errHeader: xerrors.New("oops")}
	_, err = sess.setupRelay(ctx, fake.NewAddress(1))
	require.EqualError(t, err, "failed to receive header: oops")

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

func TestSession_OnFailure(t *testing.T) {
	gw := NewRelay(&fakeStream{}, fake.NewAddress(0), fake.NewContext(), fakeConnection{}, make(metadata.MD))

	sess := &session{
		table:   fakeTable{},
		queue:   newNonBlockingQueue(),
		gateway: gw,
	}

	ctx := context.Background()

	var err error
	cb := func(e error) { err = e }

	sess.onFailure(ctx, fake.NewAddress(0), fakePkt{}, cb)

	sess.table = fakeTable{errFail: xerrors.New("oops")}
	sess.onFailure(ctx, fake.NewAddress(0), fakePkt{}, cb)
	require.EqualError(t, err, "no route to fake.Address[0]: oops")

	sess.table = fakeTable{err: xerrors.New("oops")}
	sess.onFailure(ctx, fake.NewAddress(0), fakePkt{dest: fake.NewAddress(1)}, cb)
	require.EqualError(t, err, "no route to fake.Address[400]: oops")
}

func TestRelay_Send(t *testing.T) {
	r := &unicastRelay{
		md:   make(metadata.MD),
		conn: fakeConnection{},
	}

	ack, err := r.Send(context.Background(), fakePkt{})
	require.NoError(t, err)
	require.NotNil(t, ack)

	_, err = r.Send(context.Background(), fakePkt{err: xerrors.New("oops")})
	require.EqualError(t, err, "failed to serialize: oops")

	r.conn = fakeConnection{err: xerrors.New("oops")}
	_, err = r.Send(context.Background(), fakePkt{})
	require.EqualError(t, err, "client: oops")
}

func TestRelay_Close(t *testing.T) {
	r := &unicastRelay{
		stream: &fakeStream{},
	}

	err := r.Close()
	require.NoError(t, err)

	r.stream = &fakeStream{err: xerrors.New("oops")}
	err = r.Close()
	require.EqualError(t, err, "failed to close stream: oops")
}

func TestStreamRelay_Send(t *testing.T) {
	r := &streamRelay{
		stream: &fakeStream{},
	}

	ack, err := r.Send(context.Background(), fakePkt{})
	require.NoError(t, err)
	require.Empty(t, ack.Errors)

	_, err = r.Send(context.Background(), fakePkt{err: xerrors.New("oops")})
	require.EqualError(t, err, "failed to serialize: oops")
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
	s.calls.Add("recv")
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
	s.calls.Add("send", p)
	return s.err
}

func (s *fakeStream) CloseSend() error {
	return s.err
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

type fakePktFac struct {
	router.PacketFactory

	err error
}

func (fac fakePktFac) PacketOf(serde.Context, []byte) (router.Packet, error) {
	return fakePkt{dest: fake.NewAddress(0)}, fac.err
}

type fakeTable struct {
	router.RoutingTable

	route   mino.Address
	err     error
	errFail error
}

func (t fakeTable) Make(mino.Address, []mino.Address, []byte) router.Packet {
	return fakePkt{dest: fake.NewAddress(0)}
}

func (t fakeTable) Prelude(mino.Address) router.Handshake {
	return fakeHandshake{err: t.err}
}

func (t fakeTable) Forward(router.Packet) (router.Routes, router.Voids) {
	voids := make(router.Voids)
	if t.err != nil {
		voids[fake.NewAddress(400)] = t.err
	}

	routes := router.Routes{t.route: fakePkt{}}

	return routes, voids
}

func (t fakeTable) OnFailure(mino.Address) error {
	return t.errFail
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
	errHeader error
}

func (mgr fakeConnMgr) Acquire(mino.Address) (grpc.ClientConnInterface, error) {
	conn := fakeConnection{
		msg:       mgr.msg,
		err:       mgr.errConn,
		errStream: mgr.errStream,
		errRecv:   mgr.errRecv,
		errHeader: mgr.errHeader,
	}

	return conn, mgr.err
}

func (mgr fakeConnMgr) Release(mino.Address) {}

type fakeConnection struct {
	grpc.ClientConnInterface
	msg       *ptypes.Packet
	ack       *ptypes.Ack
	err       error
	errStream error
	errRecv   error
	errHeader error
}

func (conn fakeConnection) Invoke(ctx context.Context,
	method string, args interface{}, reply interface{}, opts ...grpc.CallOption) error {

	if conn.ack != nil {
		*(reply.(*ptypes.Ack)) = *conn.ack
	}

	return conn.err
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

	stream := &fakeClientStream{
		ch:        ch,
		err:       conn.errStream,
		errRecv:   conn.errRecv,
		errHeader: conn.errHeader,
	}

	return stream, conn.err
}

type fakeClientStream struct {
	grpc.ClientStream
	ch        chan *ptypes.Packet
	err       error
	errRecv   error
	errHeader error
}

func (str *fakeClientStream) Context() context.Context {
	return context.Background()
}

func (str *fakeClientStream) Header() (metadata.MD, error) {
	return make(metadata.MD), str.errHeader
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

func (str *fakeClientStream) CloseSend() error {
	return nil
}
