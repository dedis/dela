package minogrpc

import (
	context "context"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc/ptypes"
	"go.dedis.ch/dela/mino/minogrpc/session"
	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/mino/router/tree"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestRPC_Call(t *testing.T) {
	rpc := &RPC{
		factory: fake.MessageFactory{},
		overlay: &overlay{
			connMgr: fakeConnMgr{},
			context: json.NewContext(),
		},
	}

	ctx := context.Background()
	addrs := []mino.Address{session.NewAddress("A"), session.NewAddress("B")}

	msgs, err := rpc.Call(ctx, fake.Message{}, mino.NewAddresses(addrs...))
	require.NoError(t, err)

	for i := 0; i < 2; i++ {
		msg, more := <-msgs
		require.True(t, more)
		require.NotNil(t, msg)

		reply, err := msg.GetMessageOrError()
		require.NoError(t, err)
		require.Equal(t, fake.Message{}, reply)

		isA := msg.GetFrom().Equal(session.NewAddress("A"))
		isB := msg.GetFrom().Equal(session.NewAddress("B"))
		require.True(t, isA || isB)
	}

	_, more := <-msgs
	require.False(t, more)
}

func TestRPC_EmptyAnswer_Call(t *testing.T) {
	rpc := &RPC{
		overlay: &overlay{
			connMgr: fakeConnMgr{empty: true},
			context: json.NewContext(),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addrs := mino.NewAddresses(session.NewAddress(""))

	msgs, err := rpc.Call(ctx, fake.Message{}, addrs)
	require.NoError(t, err)

	_, more := <-msgs
	require.False(t, more)
}

func TestRPC_FailSerialize_Call(t *testing.T) {
	rpc := &RPC{
		overlay: &overlay{
			context: fake.NewBadContext(),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := rpc.Call(ctx, fake.Message{}, mino.NewAddresses())
	require.EqualError(t, err, fake.Err("while serializing"))
}

func TestRPC_FailConn_Call(t *testing.T) {
	rpc := &RPC{
		factory: fake.MessageFactory{},
		overlay: &overlay{
			connMgr: fakeConnMgr{err: fake.GetError()},
			context: json.NewContext(),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addrs := mino.NewAddresses(session.NewAddress(""))

	msgs, err := rpc.Call(ctx, fake.Message{}, addrs)
	require.NoError(t, err)

	msg := <-msgs
	_, err = msg.GetMessageOrError()
	require.EqualError(t, err, fake.Err("failed to get client conn"))
}

func TestRPC_BadConn_Call(t *testing.T) {
	rpc := &RPC{
		factory: fake.MessageFactory{},
		overlay: &overlay{
			connMgr: fakeConnMgr{errConn: fake.GetError()},
			context: json.NewContext(),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addrs := mino.NewAddresses(session.NewAddress(""))

	msgs, err := rpc.Call(ctx, fake.Message{}, addrs)
	require.NoError(t, err)

	msg := <-msgs
	_, err = msg.GetMessageOrError()
	require.EqualError(t, err, fake.Err("failed to call client"))
}

func TestRPC_FailDeserialize_Call(t *testing.T) {
	rpc := &RPC{
		factory: fake.NewBadMessageFactory(),
		overlay: &overlay{
			connMgr: fakeConnMgr{},
			context: json.NewContext(),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addrs := mino.NewAddresses(session.NewAddress(""))

	msgs, err := rpc.Call(ctx, fake.Message{}, addrs)
	require.NoError(t, err)

	msg := <-msgs
	_, err = msg.GetMessageOrError()
	require.EqualError(t, err, fake.Err("couldn't unmarshal payload"))
}

func TestRPC_Stream(t *testing.T) {
	addrs := []mino.Address{session.NewAddress("A"), session.NewAddress("B")}
	calls := &fake.Call{}

	rpc := &RPC{
		overlay: &overlay{
			closer:      new(sync.WaitGroup),
			myAddr:      session.NewAddress("C"),
			router:      tree.NewRouter(addressFac),
			addrFactory: addressFac,
			connMgr:     fakeConnMgr{calls: calls},
			context:     json.NewContext(),
		},
		factory: fake.MessageFactory{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Send to multiple addresses.
	out, in, err := rpc.Stream(ctx, mino.NewAddresses(addrs...))
	require.NoError(t, err)

	out.Send(fake.Message{}, session.NewOrchestratorAddress(session.NewAddress("C")), addrs[1])
	in.Recv(ctx)

	cancel()
	rpc.overlay.closer.Wait()
	require.Equal(t, 2, calls.Len())
	require.Equal(t, addrs[0], calls.Get(1, 1))
	require.Equal(t, "release", calls.Get(1, 0))

	// Only one is contacted, and the context is canceled.
	out, in, err = rpc.Stream(ctx, mino.NewAddresses(addrs[0]))
	require.NoError(t, err)

	out.Send(fake.Message{}, addrs[0])
	_, _, err = in.Recv(ctx)
	require.Equal(t, context.Canceled, err)

	rpc.overlay.closer.Wait()
}

func TestRPC_EmptyPlayers_Stream(t *testing.T) {
	rpc := &RPC{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, _, err := rpc.Stream(ctx, mino.NewAddresses())
	require.EqualError(t, err, "empty list of addresses")
}

func TestRPC_FailRtingTable_Stream(t *testing.T) {
	rpc := &RPC{
		overlay: &overlay{
			router: badRouter{},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, _, err := rpc.Stream(ctx, mino.NewAddresses(session.NewAddress("")))
	require.EqualError(t, err, fake.Err("routing table failed"))
}

func TestRPC_BadAddress_Stream(t *testing.T) {
	rpc := &RPC{
		overlay: &overlay{
			router: tree.NewRouter(addressFac),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addrs := mino.NewAddresses(session.NewAddress(""), fake.NewBadAddress())

	_, _, err := rpc.Stream(ctx, addrs)
	require.EqualError(t, err, fake.Err("while marshaling address"))
}

func TestRPC_BadGateway_Stream(t *testing.T) {
	rpc := &RPC{
		overlay: &overlay{
			router:  tree.NewRouter(addressFac),
			connMgr: fakeConnMgr{err: fake.GetError()},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addrs := mino.NewAddresses(session.NewAddress(""))

	_, _, err := rpc.Stream(ctx, addrs)
	require.EqualError(t, err, fake.Err("gateway connection failed"))
}

func TestRPC_BadConn_Stream(t *testing.T) {
	calls := fake.NewCall()

	rpc := &RPC{
		overlay: &overlay{
			router:  tree.NewRouter(addressFac),
			connMgr: fakeConnMgr{errConn: fake.GetError(), calls: calls},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addrs := mino.NewAddresses(session.NewAddress(""))

	_, _, err := rpc.Stream(ctx, addrs)
	require.EqualError(t, err, fake.Err("failed to open stream"))
	require.Equal(t, 2, calls.Len())
	require.Equal(t, "release", calls.Get(1, 0))
}

func TestRPC_BadStream_Stream(t *testing.T) {
	calls := fake.NewCall()

	rpc := &RPC{
		overlay: &overlay{
			router:  tree.NewRouter(addressFac),
			connMgr: fakeConnMgr{errStream: fake.GetError(), calls: calls},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addrs := mino.NewAddresses(session.NewAddress(""))

	_, _, err := rpc.Stream(ctx, addrs)
	require.EqualError(t, err, fake.Err("failed to receive header"))
	require.Equal(t, 2, calls.Len())
	require.Equal(t, "release", calls.Get(1, 0))
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeClientStream struct {
	grpc.ClientStream
	init *ptypes.Packet
	ch   chan *ptypes.Packet
	err  error
}

func (str *fakeClientStream) Context() context.Context {
	return context.Background()
}

func (str *fakeClientStream) Header() (metadata.MD, error) {
	return make(metadata.MD), str.err
}

func (str *fakeClientStream) SendMsg(m interface{}) error {
	if str.err != nil {
		return str.err
	}

	if str.init == nil {
		str.init = m.(*ptypes.Packet)
		return nil
	}

	str.ch <- m.(*ptypes.Packet)
	return nil
}

func (str *fakeClientStream) RecvMsg(m interface{}) error {
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

type fakeConnection struct {
	grpc.ClientConnInterface
	resp      interface{}
	empty     bool
	err       error
	errStream error
}

func (conn fakeConnection) Invoke(ctx context.Context, m string, arg interface{},
	resp interface{}, opts ...grpc.CallOption) error {

	if conn.empty {
		return conn.err
	}

	switch msg := resp.(type) {
	case *ptypes.Message:
		*msg = ptypes.Message{
			Payload: []byte(`{}`),
		}
	case *ptypes.JoinResponse:
		*msg = conn.resp.(ptypes.JoinResponse)
	default:
	}

	return conn.err
}

func (conn fakeConnection) NewStream(ctx context.Context, desc *grpc.StreamDesc,
	m string, opts ...grpc.CallOption) (grpc.ClientStream, error) {

	ch := make(chan *ptypes.Packet, 1)

	go func() {
		<-ctx.Done()
		close(ch)
	}()

	return &fakeClientStream{ch: ch, err: conn.errStream}, conn.err
}

type fakeConnMgr struct {
	session.ConnectionManager
	resp      interface{}
	empty     bool
	len       int
	calls     *fake.Call
	err       error
	errConn   error
	errStream error
}

func (f fakeConnMgr) Len() int {
	return f.len
}

func (f fakeConnMgr) Acquire(addr mino.Address) (grpc.ClientConnInterface, error) {
	f.calls.Add("acquire", addr)

	conn := fakeConnection{
		empty:     f.empty,
		resp:      f.resp,
		err:       f.errConn,
		errStream: f.errStream,
	}

	return conn, f.err
}

func (f fakeConnMgr) Release(addr mino.Address) {
	f.calls.Add("release", addr)
}

type badRouter struct {
	router.Router

	errFac bool
}

func (r badRouter) GetHandshakeFactory() router.HandshakeFactory {
	if r.errFac {
		return fakeHandshakeFactory{err: fake.GetError()}
	}

	return fakeHandshakeFactory{}
}

func (badRouter) New(mino.Players, mino.Address) (router.RoutingTable, error) {
	return nil, fake.GetError()
}

func (badRouter) GenerateTableFrom(router.Handshake) (router.RoutingTable, error) {
	return nil, fake.GetError()
}

type fakeHandshakeFactory struct {
	router.HandshakeFactory

	err error
}

func (fac fakeHandshakeFactory) HandshakeOf(serde.Context, []byte) (router.Handshake, error) {
	return nil, fac.err
}
