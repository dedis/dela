package minogrpc

import (
	context "context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/router/tree"
	"go.dedis.ch/dela/serde/json"
	"google.golang.org/grpc"
)

func TestRPC_Call(t *testing.T) {
	rpc := &RPC{
		factory: fake.MessageFactory{},
		overlay: overlay{
			me:          fake.NewAddress(1),
			connFactory: fakeConnFactory{},
			context:     json.NewContext(),
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
		overlay: overlay{
			me:          addrs[0],
			router:      tree.NewRouter(NewMemship([]mino.Address{fake.NewAddress(0)}), 1),
			addrFactory: AddressFactory{},
			connFactory: fakeConnFactory{},
			context:     json.NewContext(),
		},
		factory: fake.MessageFactory{},
	}

	out, in, err := rpc.Stream(context.Background(), mino.NewAddresses(addrs...))
	require.NoError(t, err)

	out.Send(fake.Message{}, newRootAddress())
	in.Recv(context.Background())
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeClientStream struct {
	grpc.ClientStream
	init *Packet
	ch   chan *Packet
	err  error
}

func (str *fakeClientStream) SendMsg(m interface{}) error {
	if str.err != nil {
		return str.err
	}

	if str.init == nil {
		str.init = m.(*Packet)
		return nil
	}

	str.ch <- m.(*Packet)
	return nil
}

func (str *fakeClientStream) RecvMsg(m interface{}) error {
	msg, more := <-str.ch
	if !more {
		return io.EOF
	}

	*(m.(*Packet)) = *msg
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
	case *Message:
		*msg = Message{
			Payload: []byte(`{}`),
		}
	case *JoinResponse:
		*msg = conn.resp.(JoinResponse)
	default:
	}

	return conn.err
}

func (conn fakeConnection) NewStream(ctx context.Context, desc *grpc.StreamDesc,
	m string, opts ...grpc.CallOption) (grpc.ClientStream, error) {

	ch := make(chan *Packet, 1)

	go func() {
		<-ctx.Done()
		close(ch)
	}()

	return &fakeClientStream{ch: ch, err: conn.errStream}, conn.err
}

type fakeConnFactory struct {
	ConnectionFactory
	resp      interface{}
	err       error
	errConn   error
	errStream error
}

func (f fakeConnFactory) FromAddress(mino.Address) (grpc.ClientConnInterface, error) {
	conn := fakeConnection{
		resp:      f.resp,
		err:       f.errConn,
		errStream: f.errStream,
	}

	return conn, f.err
}
