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
			me:      fake.NewAddress(1),
			connMgr: fakeConnMgr{},
			context: json.NewContext(),
		},
	}

	ctx := context.Background()
	addrs := []mino.Address{address{"A"}, address{"B"}}

	msgs, err := rpc.Call(ctx, fake.Message{}, mino.NewAddresses(addrs...))
	require.NoError(t, err)

	for i := 0; i < 2; i++ {
		msg, more := <-msgs
		require.True(t, more)
		require.NotNil(t, msg)

		reply, err := msg.GetMessageOrError()
		require.NoError(t, err)
		require.Equal(t, fake.Message{}, reply)
		require.True(t, msg.GetFrom().Equal(address{"A"}) || msg.GetFrom().Equal(address{"B"}))
	}

	_, more := <-msgs
	require.False(t, more)

	// Test if the distant handler does not return an answer to have the channel
	// closed without anything coming into it.
	rpc.overlay.connMgr = fakeConnMgr{empty: true}
	msgs, err = rpc.Call(ctx, fake.Message{}, mino.NewAddresses(addrs...))
	require.NoError(t, err)

	_, more = <-msgs
	require.False(t, more)

	_, err = rpc.Call(ctx, fake.NewBadPublicKey(), mino.NewAddresses())
	require.EqualError(t, err, fake.Err("failed to marshal msg"))

	rpc.overlay.me = fake.NewBadAddress()
	_, err = rpc.Call(ctx, fake.Message{}, mino.NewAddresses())
	require.EqualError(t, err, fake.Err("failed to marshal address"))

	rpc.overlay.me = fake.NewAddress(0)
	rpc.overlay.connMgr = fakeConnMgr{err: fake.GetError()}
	msgs, err = rpc.Call(ctx, fake.Message{}, mino.NewAddresses(addrs...))
	require.NoError(t, err)

	msg := <-msgs
	_, err = msg.GetMessageOrError()
	require.EqualError(t, err, fake.Err("failed to get client conn"))

	rpc.overlay.connMgr = fakeConnMgr{errConn: fake.GetError()}
	msgs, err = rpc.Call(ctx, fake.Message{}, mino.NewAddresses(addrs...))
	require.NoError(t, err)

	msg = <-msgs
	_, err = msg.GetMessageOrError()
	require.EqualError(t, err, fake.Err("failed to call client"))

	rpc.overlay.connMgr = fakeConnMgr{}
	rpc.factory = fake.NewBadMessageFactory()
	msgs, err = rpc.Call(ctx, fake.Message{}, mino.NewAddresses(addrs...))
	require.NoError(t, err)

	msg = <-msgs
	_, err = msg.GetMessageOrError()
	require.EqualError(t, err, fake.Err("couldn't unmarshal payload"))
}

func TestRPC_Stream(t *testing.T) {
	addrs := []mino.Address{address{"A"}, address{"B"}}
	calls := &fake.Call{}

	rpc := &RPC{
		overlay: &overlay{
			closer:      new(sync.WaitGroup),
			me:          address{"C"},
			router:      tree.NewRouter(AddressFactory{}),
			addrFactory: AddressFactory{},
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

	out.Send(fake.Message{}, newRootAddress(), addrs[1])
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

	_, _, err = rpc.Stream(ctx, mino.NewAddresses())
	require.EqualError(t, err, "empty list of addresses")

	rpc.overlay.router = badRouter{}
	_, _, err = rpc.Stream(ctx, mino.NewAddresses(addrs[0]))
	require.EqualError(t, err, fake.Err("routing table failed"))

	rpc.overlay.router = tree.NewRouter(AddressFactory{})
	_, _, err = rpc.Stream(ctx, mino.NewAddresses(addrs[0], fake.NewBadAddress()))
	require.EqualError(t, err, fake.Err("marshal address failed"))

	rpc.overlay.connMgr = fakeConnMgr{err: fake.GetError()}
	_, _, err = rpc.Stream(ctx, mino.NewAddresses(addrs...))
	require.EqualError(t, err, fake.Err("gateway connection failed"))

	rpc.overlay.connMgr = fakeConnMgr{errConn: fake.GetError(), calls: calls}
	_, _, err = rpc.Stream(ctx, mino.NewAddresses(addrs...))
	require.EqualError(t, err, fake.Err("failed to open stream"))
	require.Equal(t, 6, calls.Len())
	require.Equal(t, "release", calls.Get(3, 0))

	rpc.overlay.connMgr = fakeConnMgr{errStream: fake.GetError(), calls: calls}
	_, _, err = rpc.Stream(ctx, mino.NewAddresses(addrs...))
	require.EqualError(t, err, fake.Err("failed to receive header"))
	require.Equal(t, 8, calls.Len())
	require.Equal(t, "release", calls.Get(5, 0))
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

func (badRouter) New(mino.Players) (router.RoutingTable, error) {
	return nil, fake.GetError()
}

func (badRouter) TableOf(router.Handshake) (router.RoutingTable, error) {
	return nil, fake.GetError()
}

type fakeHandshakeFactory struct {
	router.HandshakeFactory

	err error
}

func (fac fakeHandshakeFactory) HandshakeOf(serde.Context, []byte) (router.Handshake, error) {
	return nil, fac.err
}
