package minogrpc

import (
	"context"
	"crypto/tls"
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
	mm, rpcs := makeInstances(t, 5)

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
		require.NoError(t, m.(*Minogrpc).GracefulClose())
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
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for closure")
	}

	cancel()

	for _, m := range mm {
		require.NoError(t, m.(*Minogrpc).GracefulClose())
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

	badCtx := metadata.NewIncomingContext(context.Background(), metadata.New(
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

func TestOverlayServer_Stream(t *testing.T) {
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

	err = overlay.Stream(fakeServerStream{ch: ch, ctx: inCtx})
	require.NoError(t, err)

	err = overlay.Stream(fakeServerStream{ctx: ctx})
	require.EqualError(t, err, "handler '' is not registered")

	inCtx = metadata.NewIncomingContext(ctx, metadata.Pairs(headerURIKey, "unknown"))
	err = overlay.Stream(fakeServerStream{ctx: inCtx})
	require.EqualError(t, err, "handler 'unknown' is not registered")

	inCtx = metadata.NewIncomingContext(ctx, metadata.Pairs(headerURIKey, "test"))
	err = overlay.Stream(fakeServerStream{ctx: inCtx, err: xerrors.New("oops")})
	require.EqualError(t, err, "failed to receive routing message: oops")

	overlay.routingFactory = badRtingFactory{}
	ch = make(chan *Envelope, 1)
	ch <- &Envelope{Message: &Message{Payload: rtingAny}}
	err = overlay.Stream(fakeServerStream{ch: ch, ctx: inCtx})
	require.EqualError(t, err, "couldn't decode routing: oops")

	overlay.routingFactory = routing.NewTreeRoutingFactory(3, AddressFactory{})
	ch = make(chan *Envelope, 1)
	ch <- &Envelope{Message: &Message{Payload: rtingAny}}
	inCtx = metadata.NewIncomingContext(ctx, metadata.Pairs(headerURIKey, "bad"))
	err = overlay.Stream(fakeServerStream{ch: ch, ctx: inCtx})
	require.EqualError(t, err, "handler failed to process: oops")
}

func TestOverlay_SetupRelays(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	authority := fake.NewAuthority(3, fake.NewSigner)

	rtingFactory := routing.NewTreeRoutingFactory(1, fake.AddressFactory{})
	rting, err := rtingFactory.FromIterator(fake.NewAddress(0), authority.AddressIterator())
	require.NoError(t, err)

	overlay := overlay{
		me:             fake.NewAddress(0),
		encoder:        encoding.NewProtoEncoder(),
		routingFactory: rtingFactory,
		connFactory:    fakeConnFactory{},
	}

	sender, _, err := overlay.setupRelays(ctx, fake.NewAddress(0), rting)
	require.NoError(t, err)
	require.Len(t, sender.clients, 2)

	overlay.connFactory = fakeConnFactory{errConn: xerrors.New("oops")}
	_, _, err = overlay.setupRelays(ctx, fake.NewAddress(0), rting)
	require.EqualError(t, err,
		"couldn't setup relay to fake.Address[1]: couldn't open relay: oops")

	overlay.connFactory = fakeConnFactory{errStream: xerrors.New("oops")}
	_, _, err = overlay.setupRelays(ctx, fake.NewAddress(0), rting)
	require.EqualError(t, err,
		"couldn't setup relay to fake.Address[1]: couldn't send routing: oops")

	overlay.connFactory = fakeConnFactory{}
	overlay.encoder = fake.BadPackAnyEncoder{}
	_, _, err = overlay.setupRelays(ctx, fake.NewAddress(0), rting)
	require.EqualError(t, err,
		"couldn't setup relay to fake.Address[1]: couldn't pack routing: fake error")
}

func TestConnectionFactory_FromAddress(t *testing.T) {
	dst, err := NewMinogrpc("127.0.0.1", 3334, nil)
	require.NoError(t, err)

	defer dst.GracefulClose()

	factory := DefaultConnectionFactory{
		certs: &sync.Map{},
		me:    fake.NewAddress(0),
	}

	factory.certs.Store(factory.me, &tls.Certificate{})
	factory.certs.Store(dst.GetAddress(), dst.GetCertificate())

	conn, err := factory.FromAddress(dst.GetAddress())
	require.NoError(t, err)
	require.NotNil(t, conn)

	conn.(*grpc.ClientConn).Close()

	_, err = factory.FromAddress(fake.NewAddress(1))
	require.EqualError(t, err, "certificate for 'fake.Address[1]' not found")

	factory.certs.Store(fake.NewAddress(1), nil)
	_, err = factory.FromAddress(fake.NewAddress(1))
	require.EqualError(t, err, "invalid certificate type '<nil>' for 'fake.Address[1]'")

	factory.certs.Delete(factory.me)
	_, err = factory.FromAddress(dst.GetAddress())
	require.EqualError(t, err, "couldn't find server 'fake.Address[0]' certificate")

	factory.certs.Store(factory.me, nil)
	_, err = factory.FromAddress(dst.GetAddress())
	require.EqualError(t, err, "invalid certificate type '<nil>' for 'fake.Address[0]'")

	factory.certs.Store(factory.me, dst.GetCertificate())
	_, err = factory.FromAddress(factory.me)
	require.EqualError(t, err, "invalid address type 'fake.Address'")
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
