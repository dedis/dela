package minogrpc

import (
	context "context"
	"testing"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/fabric/mino/minogrpc/routing"
	"google.golang.org/grpc"
)

func TestRPC_Call(t *testing.T) {
	rpc := &RPC{
		overlay: overlay{
			encoder:     encoding.NewProtoEncoder(),
			connFactory: fakeConnFactory{},
		},
	}

	addrs := []mino.Address{address{"A"}, address{"B"}}

	msgs, errs := rpc.Call(context.Background(), &empty.Empty{}, mino.NewAddresses(addrs...))
	select {
	case err := <-errs:
		t.Fatal(err)
	case msg := <-msgs:
		require.NotNil(t, msg)
	}
}

func TestRPC_Stream(t *testing.T) {
	addrs := []mino.Address{address{"A"}, address{"B"}}

	rpc := &RPC{
		overlay: overlay{
			encoder:        encoding.NewProtoEncoder(),
			me:             addrs[0],
			routingFactory: routing.NewTreeRoutingFactory(1, AddressFactory{}),
			connFactory:    fakeConnFactory{},
		},
	}

	out, in := rpc.Stream(context.Background(), mino.NewAddresses(addrs...))

	out.Send(&empty.Empty{}, newRootAddress())
	in.Recv(context.Background())
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeClientStream struct {
	grpc.ClientStream
	init *Envelope
	ch   chan *Envelope
}

func (str *fakeClientStream) SendMsg(m interface{}) error {
	if str.init == nil {
		str.init = m.(*Envelope)
		return nil
	}

	str.ch <- m.(*Envelope)
	return nil
}

func (str *fakeClientStream) RecvMsg(m interface{}) error {
	*(m.(*Envelope)) = *<-str.ch
	return nil
}

type fakeConnection struct {
	grpc.ClientConnInterface
}

func (conn fakeConnection) Invoke(ctx context.Context, m string, arg interface{},
	resp interface{}, opts ...grpc.CallOption) error {

	emptyAny, err := ptypes.MarshalAny(&empty.Empty{})
	if err != nil {
		return err
	}

	*(resp.(*Message)) = Message{
		Payload: emptyAny,
	}

	return nil
}

func (conn fakeConnection) NewStream(context.Context, *grpc.StreamDesc, string,
	...grpc.CallOption) (grpc.ClientStream, error) {

	return &fakeClientStream{ch: make(chan *Envelope, 1)}, nil
}

type fakeConnFactory struct {
	ConnectionFactory
}

func (f fakeConnFactory) FromAddress(mino.Address) (grpc.ClientConnInterface, error) {
	return fakeConnection{}, nil
}
