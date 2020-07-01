package minogrpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc/certs"
	"go.dedis.ch/dela/mino/minogrpc/routing"
	"go.dedis.ch/dela/mino/minogrpc/tokens"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
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
		err := <-sender.Send(fake.Message{}, to)
		require.NoError(t, err)

		from, msg, err := recv.Recv(context.Background())
		require.NoError(t, err)
		require.Equal(t, to, from)
		require.IsType(t, fake.Message{}, msg)
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

	resps, _ := rpcs[0].Call(ctx, fake.Message{}, authority)

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

func TestOverlayServer_Join(t *testing.T) {
	overlay := overlayServer{
		overlay: overlay{
			tokens:         fakeTokens{},
			certs:          certs.NewInMemoryStore(),
			routingFactory: routing.NewTreeRoutingFactory(3, AddressFactory{}),
			connFactory:    fakeConnFactory{},
		},
	}

	cert, err := makeCertificate()
	require.NoError(t, err)

	overlay.certs.Store(fake.NewAddress(0), cert)

	ctx := context.Background()
	req := &JoinRequest{
		Token: "abc",
		Certificate: &Certificate{
			Address: []byte{},
			Value:   cert.Leaf.Raw,
		},
	}

	resp, err := overlay.Join(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	overlay.tokens = fakeTokens{invalid: true}
	_, err = overlay.Join(ctx, req)
	require.EqualError(t, err, "token 'abc' is invalid")

	overlay.tokens = fakeTokens{}
	overlay.certs.Store(fake.NewBadAddress(), cert)
	_, err = overlay.Join(ctx, req)
	require.EqualError(t, err, "couldn't marshal address: fake error")

	overlay.certs = certs.NewInMemoryStore()
	overlay.certs.Store(fake.NewAddress(0), cert)
	overlay.connFactory = fakeConnFactory{err: xerrors.New("oops")}
	_, err = overlay.Join(ctx, req)
	require.EqualError(t, err,
		"failed to share certificate: couldn't open connection: oops")

	overlay.connFactory = fakeConnFactory{errConn: xerrors.New("oops")}
	_, err = overlay.Join(ctx, req)
	require.EqualError(t, err,
		"failed to share certificate: couldn't call share: oops")
}

func TestOverlayServer_Share(t *testing.T) {
	overlay := overlayServer{
		overlay: overlay{
			certs:          certs.NewInMemoryStore(),
			routingFactory: routing.NewTreeRoutingFactory(3, AddressFactory{}),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cert := fake.MakeCertificate(t, 1)

	resp, err := overlay.Share(ctx, &Certificate{Value: cert.Leaf.Raw})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, overlay.certs.Load(address{}))

	_, err = overlay.Share(ctx, &Certificate{})
	require.EqualError(t, err,
		"couldn't parse certificate: asn1: syntax error: sequence truncated")
}

func TestOverlayServer_Call(t *testing.T) {
	overlay := overlayServer{
		overlay: overlay{
			routingFactory: routing.NewTreeRoutingFactory(3, AddressFactory{}),
			context:        json.NewContext(),
		},
		endpoints: map[string]Endpoint{
			"test": {Handler: testCallHandler{}, Factory: fake.MessageFactory{}},
			"bad":  {Handler: mino.UnsupportedHandler{}, Factory: fake.MessageFactory{}},
			"bad2": {Handler: testCallHandler{}, Factory: fake.NewBadMessageFactory()},
		},
	}

	md := metadata.New(map[string]string{headerURIKey: "test"})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	resp, err := overlay.Call(ctx, &Message{Payload: []byte(`{}`)})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, []byte(`{}`), resp.GetPayload())

	badCtx := metadata.NewIncomingContext(context.Background(), metadata.New(
		map[string]string{headerURIKey: "unknown"},
	))
	_, err = overlay.Call(badCtx, nil)
	require.EqualError(t, err, "handler 'unknown' is not registered")

	badCtx = metadata.NewIncomingContext(context.Background(), metadata.New(
		map[string]string{headerURIKey: "bad2"},
	))
	_, err = overlay.Call(badCtx, &Message{Payload: []byte(``)})
	require.EqualError(t, err, "couldn't deserialize message: fake error")

	badCtx = metadata.NewIncomingContext(context.Background(), metadata.New(
		map[string]string{headerURIKey: "bad"},
	))
	_, err = overlay.Call(badCtx, &Message{Payload: []byte(``)})
	require.EqualError(t, err, "handler failed to process: rpc is not supported")

	overlay.context = fake.NewBadContext()
	_, err = overlay.Call(ctx, &Message{Payload: []byte(``)})
	require.EqualError(t, err, "couldn't serialize result: fake error")
}

func TestOverlayServer_Stream(t *testing.T) {
	overlay := overlayServer{
		overlay: overlay{
			routingFactory: routing.NewTreeRoutingFactory(3, AddressFactory{}),
			context:        json.NewContext(),
		},
		closer: &sync.WaitGroup{},
		endpoints: map[string]Endpoint{
			"test": {Handler: testStreamHandler{skip: true}},
			"bad":  {Handler: testStreamHandler{skip: true, err: xerrors.New("oops")}},
		},
	}

	rting, err := routing.NewTreeRouting(mino.NewAddresses(address{"A"}))
	require.NoError(t, err)

	data, err := rting.Serialize(json.NewContext())
	require.NoError(t, err)

	ch := make(chan *Envelope, 1)
	ch <- &Envelope{Message: &Message{Payload: data}}

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
	overlay.context = fake.NewBadContext()
	ch = make(chan *Envelope, 1)
	ch <- &Envelope{Message: &Message{Payload: data}}
	err = overlay.Stream(fakeServerStream{ch: ch, ctx: inCtx})
	require.EqualError(t, err, "couldn't deserialize routing: oops")

	overlay.context = json.NewContext()
	overlay.routingFactory = routing.NewTreeRoutingFactory(3, AddressFactory{})
	ch = make(chan *Envelope, 1)
	ch <- &Envelope{Message: &Message{Payload: data}}
	inCtx = metadata.NewIncomingContext(ctx, metadata.Pairs(headerURIKey, "bad"))
	err = overlay.Stream(fakeServerStream{ch: ch, ctx: inCtx})
	require.EqualError(t, err, "handler failed to process: oops")
}

func TestOverlay_Join(t *testing.T) {
	cert, err := makeCertificate()
	require.NoError(t, err)

	overlay := overlay{
		me:             fake.NewAddress(0),
		certs:          fakeCerts{},
		routingFactory: routing.NewTreeRoutingFactory(3, AddressFactory{}),
		connFactory: fakeConnFactory{
			resp: JoinResponse{Peers: []*Certificate{{Value: cert.Leaf.Raw}}},
		},
	}

	err = overlay.Join("", "", nil)
	require.NoError(t, err)

	overlay.routingFactory = routing.NewTreeRoutingFactory(3, fake.AddressFactory{})
	err = overlay.Join("", "", nil)
	require.EqualError(t, err, "invalid address type 'fake.Address'")

	overlay.routingFactory = routing.NewTreeRoutingFactory(3, AddressFactory{})
	overlay.me = fake.NewBadAddress()
	err = overlay.Join("", "", nil)
	require.EqualError(t, err, "couldn't marshal own address: fake error")

	overlay.me = fake.NewAddress(0)
	overlay.certs = fakeCerts{err: xerrors.New("oops")}
	err = overlay.Join("", "", nil)
	require.EqualError(t, err, "couldn't fetch distant certificate: oops")

	overlay.certs = fakeCerts{}
	overlay.connFactory = fakeConnFactory{err: xerrors.New("oops")}
	err = overlay.Join("", "", nil)
	require.EqualError(t, err, "couldn't open connection: oops")

	overlay.connFactory = fakeConnFactory{resp: JoinResponse{}, errConn: xerrors.New("oops")}
	err = overlay.Join("", "", nil)
	require.EqualError(t, err, "couldn't call join: oops")

	overlay.connFactory = fakeConnFactory{resp: JoinResponse{Peers: []*Certificate{{}}}}
	err = overlay.Join("", "", nil)
	require.EqualError(t, err,
		"couldn't parse certificate: asn1: syntax error: sequence truncated")
}

func TestOverlay_SetupRelays(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	authority := fake.NewAuthority(3, fake.NewSigner)

	rtingFactory := routing.NewTreeRoutingFactory(1, fake.AddressFactory{})
	rting, err := rtingFactory.Make(fake.NewAddress(0), authority)
	require.NoError(t, err)

	overlay := overlay{
		me:             fake.NewAddress(0),
		routingFactory: rtingFactory,
		connFactory:    fakeConnFactory{},
		context:        json.NewContext(),
	}

	sender, _, err := overlay.setupRelays(ctx, fake.NewAddress(0), rting, fake.MessageFactory{})
	require.NoError(t, err)
	require.Len(t, sender.clients, 2)

	overlay.connFactory = fakeConnFactory{errConn: xerrors.New("oops")}
	_, _, err = overlay.setupRelays(ctx, fake.NewAddress(0), rting, fake.MessageFactory{})
	require.EqualError(t, err,
		"couldn't setup relay to fake.Address[1]: couldn't open relay: oops")

	overlay.connFactory = fakeConnFactory{errStream: xerrors.New("oops")}
	_, _, err = overlay.setupRelays(ctx, fake.NewAddress(0), rting, fake.MessageFactory{})
	require.EqualError(t, err,
		"couldn't setup relay to fake.Address[1]: couldn't send routing: oops")

	overlay.connFactory = fakeConnFactory{}
	overlay.context = fake.NewBadContext()
	_, _, err = overlay.setupRelays(ctx, fake.NewAddress(0), rting, fake.MessageFactory{})
	require.EqualError(t, err,
		"couldn't setup relay to fake.Address[1]: couldn't pack routing: fake error")
}

func TestConnectionFactory_FromAddress(t *testing.T) {
	dst, err := NewMinogrpc("127.0.0.1", 3334, nil)
	require.NoError(t, err)

	defer dst.GracefulClose()

	factory := DefaultConnectionFactory{
		certs: certs.NewInMemoryStore(),
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

	factory.certs.Delete(factory.me)
	_, err = factory.FromAddress(dst.GetAddress())
	require.EqualError(t, err, "couldn't find server 'fake.Address[0]' certificate")

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

		rpc, err := m.MakeRPC("test", testStreamHandler{}, fake.MessageFactory{})
		require.NoError(t, err)

		mm[i] = m
		rpcs[i] = rpc

		for _, k := range mm[:i] {
			km := k.(*Minogrpc)

			m.GetCertificateStore().Store(k.GetAddress(), km.GetCertificate())
			km.GetCertificateStore().Store(m.GetAddress(), m.GetCertificate())
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

func (h testCallHandler) Process(req mino.Request) (serde.Message, error) {
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

type fakeTokens struct {
	tokens.Holder
	invalid bool
}

func (t fakeTokens) Verify(string) bool {
	return !t.invalid
}

type fakeCerts struct {
	certs.Storage
	err error
}

func (s fakeCerts) Store(mino.Address, *tls.Certificate) {

}

func (s fakeCerts) Load(mino.Address) *tls.Certificate {
	return &tls.Certificate{Leaf: &x509.Certificate{Raw: []byte{0x89}}}
}

func (s fakeCerts) Fetch(certs.Dialable, []byte) error {
	return s.err
}
