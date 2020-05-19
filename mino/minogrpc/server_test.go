package minogrpc

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/internal/testing/fake"
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/fabric/mino/minogrpc/routing"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestIntegration_BasicLifecycle_Stream(t *testing.T) {
	mm, rpcs := makeInstances(t, 10)

	authority := fake.NewAuthorityFromMino(fake.NewSigner, mm...)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sender, recv, err := rpcs[0].Stream(ctx, authority)
	require.NoError(t, err)

	iter := authority.AddressIterator()
	for iter.HasNext() {
		to := iter.GetNext()
		err := <-sender.Send(&empty.Empty{}, to)
		require.NoError(t, err)

		from, msg, err := recv.Recv(context.Background())
		require.NoError(t, err)
		require.Equal(t, to, from)
		require.IsType(t, (*empty.Empty)(nil), msg)
	}

	// Start the shutdown procedure.
	cancel()

	for _, m := range mm {
		// This makes sure that the relay handlers have been closed by the
		// context.
		m.(*Minogrpc).GracefulClose()
	}
}

func TestIntegration_Basic_Call(t *testing.T) {
	mm, rpcs := makeInstances(t, 10)

	authority := fake.NewAuthorityFromMino(fake.NewSigner, mm...)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resps, _ := rpcs[0].Call(ctx, &empty.Empty{}, authority)

	select {
	case <-resps:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for closure")
	}

	cancel()

	for _, m := range mm {
		m.(*Minogrpc).GracefulClose()
	}
}

func TestOverlayServer_Call(t *testing.T) {
	overlay := overlayServer{
		overlay: overlay{
			encoder:        encoding.NewProtoEncoder(),
			routingFactory: routing.NewTreeRoutingFactory(3, AddressFactory{}),
		},
		handlers: map[string]mino.Handler{
			"test": testCallHandler{},
			"bad":  mino.UnsupportedHandler{},
		},
	}

	md := metadata.New(map[string]string{headerURIKey: "test"})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	emptyAny, err := ptypes.MarshalAny(&empty.Empty{})
	require.NoError(t, err)

	resp, err := overlay.Call(ctx, &Message{Payload: emptyAny})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.True(t, ptypes.Is(resp.GetPayload(), (*empty.Empty)(nil)))

	_, err = overlay.Call(context.Background(), nil)
	require.EqualError(t, err, "header not found in provided context")

	badCtx := metadata.NewIncomingContext(context.Background(), metadata.MD{})
	_, err = overlay.Call(badCtx, nil)
	require.EqualError(t, err, "'apiuri' not found in context header")

	badCtx = metadata.NewIncomingContext(context.Background(), metadata.New(
		map[string]string{headerURIKey: "unknown"},
	))
	_, err = overlay.Call(badCtx, nil)
	require.EqualError(t, err, "handler 'unknown' is not registered")

	overlay.encoder = fake.BadUnmarshalDynEncoder{}
	_, err = overlay.Call(ctx, &Message{Payload: emptyAny})
	require.EqualError(t, err, "couldn't unmarshal message: fake error")

	badCtx = metadata.NewIncomingContext(context.Background(), metadata.New(
		map[string]string{headerURIKey: "bad"},
	))
	overlay.encoder = encoding.NewProtoEncoder()
	_, err = overlay.Call(badCtx, &Message{Payload: emptyAny})
	require.EqualError(t, err, "handler failed to process: rpc is not supported")

	overlay.encoder = fake.BadMarshalAnyEncoder{}
	_, err = overlay.Call(ctx, &Message{Payload: emptyAny})
	require.EqualError(t, err, "couldn't marshal result: fake error")
}

func TestOverlayServer_Relay(t *testing.T) {
	overlay := overlayServer{
		overlay: overlay{
			routingFactory: routing.NewTreeRoutingFactory(3, AddressFactory{}),
		},
		closer: &sync.WaitGroup{},
		handlers: map[string]mino.Handler{
			"test": testStreamHandler{skip: true},
			"bad":  testStreamHandler{skip: true, err: xerrors.New("oops")},
		},
	}

	rting, err := overlay.routingFactory.FromIterator(address{"A"}, mino.NewAddresses().AddressIterator())
	require.NoError(t, err)

	rtingAny, err := encoding.NewProtoEncoder().PackAny(rting)
	require.NoError(t, err)

	ch := make(chan *Envelope, 1)
	ch <- &Envelope{Message: &Message{Payload: rtingAny}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	inCtx := metadata.NewIncomingContext(ctx, metadata.Pairs(headerURIKey, "test"))

	err = overlay.Relay(fakeServerStream{ch: ch, ctx: inCtx})
	require.NoError(t, err)

	err = overlay.Relay(fakeServerStream{ctx: context.Background()})
	require.EqualError(t, err, "handler '' is not registered")

	inCtx = metadata.NewIncomingContext(ctx, metadata.Pairs(headerURIKey, "unknown"))
	err = overlay.Relay(fakeServerStream{ctx: inCtx})
	require.EqualError(t, err, "handler 'unknown' is not registered")

	inCtx = metadata.NewIncomingContext(ctx, metadata.Pairs(headerURIKey, "test"))
	err = overlay.Relay(fakeServerStream{ctx: inCtx, err: xerrors.New("oops")})
	require.EqualError(t, err, "failed to receive routing message: oops")

	overlay.routingFactory = badRtingFactory{}
	ch <- &Envelope{Message: &Message{Payload: rtingAny}}
	err = overlay.Relay(fakeServerStream{ch: ch, ctx: inCtx})
	require.EqualError(t, err, "couldn't decode routing: oops")

	overlay.routingFactory = routing.NewTreeRoutingFactory(3, AddressFactory{})
	ch <- &Envelope{Message: &Message{Payload: rtingAny}}
	inCtx = metadata.NewIncomingContext(ctx, metadata.Pairs(headerURIKey, "bad"))
	err = overlay.Relay(fakeServerStream{ch: ch, ctx: inCtx})
	require.EqualError(t, err, "handler failed to process: oops")
}

// -----------------------------------------------------------------------------
// Utility functions

func makeInstances(t *testing.T, n int) ([]mino.Mino, []mino.RPC) {
	rtingFactory := routing.NewTreeRoutingFactory(2, AddressFactory{})
	mm := make([]mino.Mino, n)
	rpcs := make([]mino.RPC, n)
	for i := range mm {
		m, err := NewMinogrpc("127.0.0.1", 3000+uint16(i), rtingFactory)
		require.NoError(t, err)

		rpc, err := m.MakeRPC("test", testStreamHandler{})
		require.NoError(t, err)

		mm[i] = m
		rpcs[i] = rpc

		for _, k := range mm[:i] {
			km := k.(*Minogrpc)

			m.AddCertificate(k.GetAddress(), km.GetCertificate())
			km.AddCertificate(m.GetAddress(), m.GetCertificate())
		}
	}

	return mm, rpcs
}

type testStreamHandler struct {
	mino.UnsupportedHandler
	skip bool
	err  error
}

// Stream implements mino.Handler. It implements a simple receiver that will
// return the message received and close.
func (h testStreamHandler) Stream(out mino.Sender, in mino.Receiver) error {
	if h.skip {
		return h.err
	}

	from, msg, err := in.Recv(context.Background())
	if err != nil {
		return err
	}

	err = <-out.Send(msg, from)
	if err != nil {
		return err
	}

	return nil
}

type testCallHandler struct {
	mino.UnsupportedHandler
}

func (h testCallHandler) Process(req mino.Request) (proto.Message, error) {
	return req.Message, nil
}

type fakeServerStream struct {
	grpc.ServerStream
	ch  chan *Envelope
	ctx context.Context
	err error
}

func (s fakeServerStream) Context() context.Context {
	return s.ctx
}

func (s fakeServerStream) Send(m *Envelope) error {
	s.ch <- m
	return nil
}

func (s fakeServerStream) Recv() (*Envelope, error) {
	if s.err != nil {
		return nil, s.err
	}

	env := <-s.ch
	return env, nil
}
