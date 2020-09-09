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
	"go.dedis.ch/dela/mino/router/tree"
	"go.dedis.ch/dela/serde/json"
	"google.golang.org/grpc"
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

	addrs := []mino.Address{address{"A"}, address{"B"}}

	msgs, err := rpc.Call(context.Background(), fake.Message{}, mino.NewAddresses(addrs...))
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
}

func TestRPC_Stream(t *testing.T) {
	addrs := []mino.Address{address{"A"}, address{"B"}}

	rpc := &RPC{
		overlay: &overlay{
			closer:      new(sync.WaitGroup),
			me:          addrs[0],
			router:      tree.NewRouter(1, AddressFactory{}),
			addrFactory: AddressFactory{},
			connMgr:     fakeConnMgr{},
			context:     json.NewContext(),
		},
		factory: fake.MessageFactory{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out, in, err := rpc.Stream(ctx, mino.NewAddresses(addrs...))
	require.NoError(t, err)

	out.Send(fake.Message{}, newRootAddress())
	in.Recv(ctx)

	cancel()
	rpc.overlay.closer.Wait()
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeClientStream struct {
	grpc.ClientStream
	init *ptypes.Packet
	ch   chan *ptypes.Packet
	err  error
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

type fakeConnection struct {
	grpc.ClientConnInterface
	resp      interface{}
	err       error
	errStream error
}

func (conn fakeConnection) Invoke(ctx context.Context, m string, arg interface{},
	resp interface{}, opts ...grpc.CallOption) error {

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
	err       error
	errConn   error
	errStream error
}

func (f fakeConnMgr) Acquire(mino.Address) (grpc.ClientConnInterface, error) {
	conn := fakeConnection{
		resp:      f.resp,
		err:       f.errConn,
		errStream: f.errStream,
	}

	return conn, f.err
}

func (f fakeConnMgr) Release(mino.Address) {}
