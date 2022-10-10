package session

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

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

func TestSessionNew(t *testing.T) {
	curr := os.Getenv(traffic.EnvVariable)
	defer os.Setenv(traffic.EnvVariable, curr)

	os.Setenv(traffic.EnvVariable, "log")
	sess := NewSession(nil, fake.NewAddress(999), nil, nil, fake.NewContext(), nil)
	require.NotNil(t, sess.(*session).traffic)

	os.Setenv(traffic.EnvVariable, "print")
	sess = NewSession(nil, fake.NewAddress(999), nil, nil, fake.NewContext(), nil)
	require.NotNil(t, sess.(*session).traffic)

	os.Unsetenv(traffic.EnvVariable)
	sess = NewSession(nil, fake.NewAddress(999), nil, nil, fake.NewContext(), nil)
	require.Nil(t, sess.(*session).traffic)
}

func TestSession_getNumParents(t *testing.T) {
	sess := &session{}

	num := sess.GetNumParents()
	require.Equal(t, 0, num)

	sess.parents = map[mino.Address]parent{
		fake.NewAddress(0): {},
		fake.NewAddress(1): {},
	}

	num = sess.GetNumParents()
	require.Equal(t, 2, num)
}

func TestSession_Listen(t *testing.T) {
	p := &streamRelay{
		stream: &fakeStream{},
		gw:     fake.NewAddress(123),
	}

	sess := &session{
		errs: make(chan error),
		parents: map[mino.Address]parent{
			p.gw: {relay: p},
		},
	}

	ready := make(chan struct{})

	sess.Listen(p, fakeTable{}, ready)
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("expect channel to be closed")
	}

	require.Len(t, sess.parents, 0)

	sess.errs = make(chan error, 1)
	p.stream = &fakeStream{err: fake.GetError()}
	sess.Listen(p, fakeTable{}, make(chan struct{}))
	select {
	case err := <-sess.errs:
		require.EqualError(t, err, fake.Err("stream closed unexpectedly"))
	default:
		t.Fatal("expect an error")
	}
}

func TestSession_Close(t *testing.T) {
	sess := &session{errs: make(chan error)}

	sess.Close()
	select {
	case <-sess.errs:
	default:
		t.Fatal("expect channel to be closed")
	}
}

func TestSession_Passive(t *testing.T) {
	sess := &session{
		parents: make(map[mino.Address]parent),
	}

	sess.SetPassive(&streamRelay{}, fakeTable{})
	require.Len(t, sess.parents, 1)
}

func TestSession_RecvPacket(t *testing.T) {
	sess := &session{
		pktFac: fakePktFac{},
		queue:  newNonBlockingQueue(),
		parents: map[mino.Address]parent{
			fake.NewAddress(123): {
				relay: &streamRelay{stream: &fakeStream{}},
				table: fakeTable{err: xerrors.New("bad route")},
			},
		},
	}

	ack, err := sess.RecvPacket(fake.NewAddress(0), &ptypes.Packet{})
	require.NoError(t, err)
	require.NotEmpty(t, ack.Errors)
	require.Equal(t, "no route to fake.Address[400]: bad route", ack.GetErrors()[0])

	sess.pktFac = fakePktFac{err: fake.GetError()}
	_, err = sess.RecvPacket(fake.NewAddress(0), &ptypes.Packet{})
	require.EqualError(t, err, fake.Err("packet malformed"))

	sess.pktFac = fakePktFac{}
	sess.parents = nil
	_, err = sess.RecvPacket(fake.NewAddress(0), &ptypes.Packet{})
	require.EqualError(t, err, "packet is dropped (tried 0 parent-s)")
}

func TestSession_Send(t *testing.T) {
	stream := &fakeStream{calls: &fake.Call{}}
	key := fake.NewAddress(123)

	sess := &session{
		me:      fake.NewAddress(600),
		context: fake.NewContext(),
		queue:   newNonBlockingQueue(),
		relays:  make(map[mino.Address]Relay),
		parents: map[mino.Address]parent{
			key: {
				relay: &streamRelay{stream: stream},
				table: fakeTable{},
			},
		},
	}

	errs := sess.Send(fake.Message{}, newWrapAddress(fake.NewAddress(0)))
	require.NoError(t, <-errs)
	require.Equal(t, 1, stream.calls.Len())

	sess.parents[key] = parent{
		relay: &streamRelay{stream: stream},
		table: fakeTable{route: fake.NewAddress(0)},
	}
	sess.relays[fake.NewAddress(0)] = NewStreamRelay(nil, &fakeStream{}, sess.context)
	errs = sess.Send(fake.Message{}, fake.NewAddress(1))
	require.NoError(t, <-errs)

	errs = sess.Send(fake.NewBadPublicKey())
	require.EqualError(t, <-errs, fake.Err("failed to serialize msg"))
	require.NoError(t, <-errs)

	sess.queue = badQueue{}
	errs = sess.Send(fake.Message{})
	require.EqualError(t, <-errs, fake.Err("fake.Address[600] dropped the packet"))

	sess.queue = newNonBlockingQueue()
	sess.parents[key] = parent{
		relay: &streamRelay{stream: stream},
		table: fakeTable{err: fake.GetError()},
	}
	errs = sess.Send(fake.Message{})
	require.EqualError(t, <-errs, fake.Err("no route to fake.Address[400]"))
	require.NoError(t, <-errs)

	// Test when an error occurred when setting up a relay, which moves to the
	// failure handler that will then fail on the routing table.
	sess.parents[key] = parent{
		relay: &streamRelay{stream: stream},
		table: fakeTable{errFail: fake.GetError(), route: fake.NewAddress(5)},
	}
	sess.connMgr = fakeConnMgr{err: xerrors.New("blabla")}
	errs = sess.Send(fake.Message{})
	require.EqualError(t, <-errs, fake.Err("no route to fake.Address[5]"))
	require.NoError(t, <-errs)

	// Test when an error occurred when forwarding a message to a relay, which
	// moves to the failure handler that will fail on the routing table.
	sess.relays[fake.NewAddress(6)] = NewStreamRelay(
		fake.NewAddress(800),
		&fakeStream{err: fake.GetError()},
		sess.context)

	sess.parents[key] = parent{
		relay: NewStreamRelay(nil, &fakeStream{err: fake.GetError()}, sess.context),
		table: fakeTable{errFail: xerrors.New("unavailable"), route: fake.NewAddress(6)},
	}
	errs = sess.Send(fake.Message{})
	require.EqualError(t, <-errs, "no route to fake.Address[800]: unavailable")
	require.NoError(t, <-errs)

	// Test when the parent stream has closed.
	sess.me = fake.NewAddress(123)
	sess.parents[key] = parent{
		relay: NewStreamRelay(nil, &fakeStream{err: fake.GetError()}, sess.context),
		table: fakeTable{},
	}
	errs = sess.Send(fake.Message{})
	require.EqualError(t, <-errs, "session fake.Address[123] is closing: OK")

	// Test when a packet is sent but some addresses are not reachable.
	sess.parents[key] = parent{
		relay: &unicastRelay{
			stream: &fakeStream{},
			conn:   fakeConnection{ack: &ptypes.Ack{Errors: []string{"bad route"}}},
		},
		table: fakeTable{},
	}
	errs = sess.Send(fake.Message{})
	require.EqualError(t, <-errs, "bad route")

	sess.parents = map[mino.Address]parent{
		key: {table: fakeTable{empty: true}},
	}
	errs = sess.Send(fake.Message{})
	require.EqualError(t, <-errs, "packet ignored")
}

func TestSession_SetupRelay(t *testing.T) {
	sess := &session{
		connMgr: fakeConnMgr{},
		context: fake.NewContext(),
		relays:  make(map[mino.Address]Relay),
		queue:   newNonBlockingQueue(),
	}

	p := parent{
		relay: &streamRelay{stream: &fakeStream{}},
		table: fakeTable{},
	}

	relay, err := sess.setupRelay(p, fake.NewAddress(0))
	require.NoError(t, err)
	require.NotNil(t, relay)
	sess.Wait()

	sess.connMgr = fakeConnMgr{err: fake.GetError()}
	_, err = sess.setupRelay(p, fake.NewAddress(1))
	require.EqualError(t, err, fake.Err("failed to dial"))

	sess.connMgr = fakeConnMgr{errConn: fake.GetError()}
	_, err = sess.setupRelay(p, fake.NewAddress(1))
	require.EqualError(t, err, fake.Err("client"))

	sess.connMgr = fakeConnMgr{errHeader: fake.GetError()}
	_, err = sess.setupRelay(p, fake.NewAddress(1))
	require.EqualError(t, err, fake.Err("failed to receive header"))

	sess.connMgr = fakeConnMgr{}
	p.table = fakeTable{err: fake.GetError()}
	_, err = sess.setupRelay(p, fake.NewAddress(1))
	require.EqualError(t, err, fake.Err("failed to serialize handshake"))

	p.table = fakeTable{}
	sess.connMgr = fakeConnMgr{errRecv: status.Error(codes.Canceled, "")}
	_, err = sess.setupRelay(p, fake.NewAddress(1))
	require.NoError(t, err)
	sess.Wait()

	sess.connMgr = fakeConnMgr{errRecv: fake.GetError()}
	_, err = sess.setupRelay(p, fake.NewAddress(2))
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
	require.True(t, from.Equal(fake.NewAddress(700)))
	require.Equal(t, fake.Message{}, msg)

	sess.msgFac = fake.NewBadMessageFactory()
	_, _, err = sess.Recv(ctx)
	require.EqualError(t, err, fake.Err("message"))

	sess.errs <- fake.GetError()
	_, _, err = sess.Recv(ctx)
	require.EqualError(t, err, fake.Err("stream closed unexpectedly"))

	sess.errs <- nil
	_, _, err = sess.Recv(ctx)
	require.Equal(t, io.EOF, err)

	cancel()
	_, _, err = sess.Recv(ctx)
	require.Equal(t, context.Canceled, err)
}

func TestSession_OnFailure(t *testing.T) {
	sess := &session{
		queue: newNonBlockingQueue(),
	}

	p := parent{
		relay: &streamRelay{stream: &fakeStream{}},
		table: fakeTable{},
	}

	errs := make(chan error, 1)

	sess.onFailure(p, fake.NewAddress(0), fakePkt{}, errs)

	p.table = fakeTable{errFail: fake.GetError()}
	sess.onFailure(p, fake.NewAddress(0), fakePkt{}, errs)
	require.EqualError(t, <-errs, fake.Err("no route to fake.Address[0]"))

	p.table = fakeTable{err: fake.GetError()}
	sess.onFailure(p, fake.NewAddress(0), fakePkt{dest: fake.NewAddress(1)}, errs)
	require.EqualError(t, <-errs, fake.Err("no route to fake.Address[400]"))
}

func TestRelay_Send(t *testing.T) {
	r := &unicastRelay{
		md:   make(metadata.MD),
		conn: fakeConnection{},
	}

	ack, err := r.Send(context.Background(), fakePkt{})
	require.NoError(t, err)
	require.NotNil(t, ack)

	_, err = r.Send(context.Background(), fakePkt{err: fake.GetError()})
	require.EqualError(t, err, fake.Err("failed to serialize"))

	r.conn = fakeConnection{err: fake.GetError()}
	_, err = r.Send(context.Background(), fakePkt{})
	require.EqualError(t, err, fake.Err("client"))
}

func TestRelay_Close(t *testing.T) {
	r := &unicastRelay{
		stream: &fakeStream{},
	}

	err := r.Close()
	require.NoError(t, err)

	r.stream = &fakeStream{err: fake.GetError()}
	err = r.Close()
	require.EqualError(t, err, fake.Err("failed to close stream"))
}

func TestStreamRelay_Send(t *testing.T) {
	r := &streamRelay{
		stream: &fakeStream{},
	}

	ack, err := r.Send(context.Background(), fakePkt{})
	require.NoError(t, err)
	require.Empty(t, ack.Errors)

	_, err = r.Send(context.Background(), fakePkt{err: fake.GetError()})
	require.EqualError(t, err, fake.Err("failed to serialize"))
}

func TestStreamRelay_Close(t *testing.T) {
	r := &streamRelay{}

	require.NoError(t, r.Close())
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
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	return ctx
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
	dest  mino.Address
	empty bool
	err   error
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
	if p.empty {
		return nil
	}

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
	empty   bool
	err     error
	errFail error
}

func (t fakeTable) Make(mino.Address, []mino.Address, []byte) router.Packet {
	return fakePkt{dest: fake.NewAddress(0), empty: t.empty}
}

func (t fakeTable) PrepareHandshakeFor(mino.Address) router.Handshake {
	return fakeHandshake{err: t.err}
}

func (t fakeTable) Forward(router.Packet) (router.Routes, router.Voids) {
	voids := make(router.Voids)
	if t.err != nil {
		voids[fake.NewAddress(400)] = router.Void{Error: t.err}
	}

	routes := router.Routes{}
	if !t.empty {
		routes[t.route] = fakePkt{}
	}

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

type badQueue struct {
	Queue
}

func (badQueue) Push(router.Packet) error {
	return fake.GetError()
}
