package minogrpc

import (
	context "context"
	"io"
	"testing"

	"github.com/golang/protobuf/ptypes"
	any "github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/encoding"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc/routing"
	"golang.org/x/xerrors"
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

	out, in, err := rpc.Stream(context.Background(), mino.NewAddresses(addrs...))
	require.NoError(t, err)

	out.Send(&empty.Empty{}, newRootAddress())
	in.Recv(context.Background())

	rpc.overlay.routingFactory = badRtingFactory{}
	_, _, err = rpc.Stream(context.Background(), mino.NewAddresses())
	require.EqualError(t, err, "couldn't generate routing: oops")

	rpc.overlay.routingFactory = routing.NewTreeRoutingFactory(1, AddressFactory{})
	rpc.overlay.connFactory = fakeConnFactory{err: xerrors.New("oops")}
	_, _, err = rpc.Stream(context.Background(), mino.NewAddresses())
	require.EqualError(t, err,
		"couldn't setup relay: couldn't open connection: oops")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeClientStream struct {
	grpc.ClientStream
	init *Envelope
	ch   chan *Envelope
	err  error
}

func (str *fakeClientStream) SendMsg(m interface{}) error {
	if str.err != nil {
		return str.err
	}

	if str.init == nil {
		str.init = m.(*Envelope)
		return nil
	}

	str.ch <- m.(*Envelope)
	return nil
}

func (str *fakeClientStream) RecvMsg(m interface{}) error {
	msg, more := <-str.ch
	if !more {
		return io.EOF
	}

	*(m.(*Envelope)) = *msg
	return nil
}

type fakeConnection struct {
	grpc.ClientConnInterface
	err       error
	errStream error
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

func (conn fakeConnection) NewStream(ctx context.Context, desc *grpc.StreamDesc,
	m string, opts ...grpc.CallOption) (grpc.ClientStream, error) {

	ch := make(chan *Envelope, 1)

	go func() {
		<-ctx.Done()
		close(ch)
	}()

	return &fakeClientStream{ch: ch, err: conn.errStream}, conn.err
}

type fakeConnFactory struct {
	ConnectionFactory
	err       error
	errConn   error
	errStream error
}

func (f fakeConnFactory) FromAddress(mino.Address) (grpc.ClientConnInterface, error) {
	conn := fakeConnection{
		err:       f.errConn,
		errStream: f.errStream,
	}

	return conn, f.err
}

type badRtingFactory struct {
	routing.Factory
}

func (f badRtingFactory) FromAny(*any.Any) (routing.Routing, error) {
	return nil, xerrors.New("oops")
}

func (f badRtingFactory) FromIterator(mino.Address, mino.AddressIterator) (routing.Routing, error) {
	return nil, xerrors.New("oops")
}
