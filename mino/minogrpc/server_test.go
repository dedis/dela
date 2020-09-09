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
	"go.dedis.ch/dela/mino/minogrpc/ptypes"
	"go.dedis.ch/dela/mino/minogrpc/session"
	"go.dedis.ch/dela/mino/minogrpc/tokens"
	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/mino/router/tree"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestIntegration_BasicLifecycle_Stream(t *testing.T) {

	// Use with MINO_TRAFFIC=log
	// defer func() {
	// 	SaveItems("graph.dot", true, true)
	// 	SaveEvents("events.dot")
	// }()

	mm, rpcs := makeInstances(t, 6, nil)

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
	call := &fake.Call{}
	mm, rpcs := makeInstances(t, 10, call)

	authority := fake.NewAuthorityFromMino(fake.NewSigner, mm...)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resps, err := rpcs[0].Call(ctx, fake.Message{}, authority)
	require.NoError(t, err)

	for {
		select {
		case resp, more := <-resps:
			if !more {
				for _, m := range mm {
					require.NoError(t, m.(*Minogrpc).GracefulClose())
				}

				// Verify the parameter of the Process handler.
				require.Equal(t, 10, call.Len())
				for i := 0; i < 10; i++ {
					req := call.Get(i, 0).(mino.Request)
					require.Equal(t, mm[0].GetAddress(), req.Address)
				}

				return
			}

			msg, err := resp.GetMessageOrError()
			require.NoError(t, err)
			require.Equal(t, fake.Message{}, msg)

		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for closure")
		}
	}
}

func TestOverlayServer_Join(t *testing.T) {
	overlay := overlayServer{
		overlay: &overlay{
			tokens:  fakeTokens{},
			certs:   certs.NewInMemoryStore(),
			router:  tree.NewRouter(3, AddressFactory{}),
			connMgr: fakeConnMgr{},
		},
	}

	cert, err := makeCertificate()
	require.NoError(t, err)

	overlay.certs.Store(fake.NewAddress(0), cert)

	ctx := context.Background()
	req := &ptypes.JoinRequest{
		Token: "abc",
		Certificate: &ptypes.Certificate{
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
	overlay.connMgr = fakeConnMgr{err: xerrors.New("oops")}
	_, err = overlay.Join(ctx, req)
	require.EqualError(t, err,
		"failed to share certificate: couldn't open connection: oops")

	overlay.connMgr = fakeConnMgr{errConn: xerrors.New("oops")}
	_, err = overlay.Join(ctx, req)
	require.EqualError(t, err,
		"failed to share certificate: couldn't call share: oops")
}

func TestOverlayServer_Share(t *testing.T) {
	overlay := overlayServer{
		overlay: &overlay{
			certs:       certs.NewInMemoryStore(),
			router:      tree.NewRouter(3, AddressFactory{}),
			addrFactory: AddressFactory{},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cert := fake.MakeCertificate(t, 1)

	resp, err := overlay.Share(ctx, &ptypes.Certificate{Value: cert.Leaf.Raw})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, overlay.certs.Load(address{}))

	_, err = overlay.Share(ctx, &ptypes.Certificate{})
	require.EqualError(t, err,
		"couldn't parse certificate: asn1: syntax error: sequence truncated")
}

func TestOverlayServer_Call(t *testing.T) {
	overlay := overlayServer{
		overlay: &overlay{
			router:      tree.NewRouter(3, AddressFactory{}),
			context:     json.NewContext(),
			addrFactory: AddressFactory{},
		},
		endpoints: make(map[string]*Endpoint),
	}

	overlay.endpoints["test"] = &Endpoint{Handler: testHandler{}, Factory: fake.MessageFactory{}}
	overlay.endpoints["empty"] = &Endpoint{Handler: emptyHandler{}, Factory: fake.MessageFactory{}}
	overlay.endpoints["bad"] = &Endpoint{Handler: mino.UnsupportedHandler{}, Factory: fake.MessageFactory{}}
	overlay.endpoints["bad2"] = &Endpoint{Handler: testHandler{}, Factory: fake.NewBadMessageFactory()}

	md := metadata.New(map[string]string{headerURIKey: "test"})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	resp, err := overlay.Call(ctx, &ptypes.Message{Payload: []byte(`{}`)})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, []byte(`{}`), resp.GetPayload())

	ctx = metadata.NewIncomingContext(context.Background(), metadata.Pairs(headerURIKey, "empty"))
	resp, err = overlay.Call(ctx, &ptypes.Message{Payload: []byte(`{}`)})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Nil(t, resp.GetPayload())

	badCtx := metadata.NewIncomingContext(context.Background(), metadata.New(
		map[string]string{headerURIKey: "unknown"},
	))
	_, err = overlay.Call(badCtx, nil)
	require.EqualError(t, err, "handler 'unknown' is not registered")

	badCtx = metadata.NewIncomingContext(context.Background(), metadata.New(
		map[string]string{headerURIKey: "bad2"},
	))
	_, err = overlay.Call(badCtx, &ptypes.Message{Payload: []byte(``)})
	require.EqualError(t, err, "couldn't deserialize message: fake error")

	badCtx = metadata.NewIncomingContext(context.Background(), metadata.New(
		map[string]string{headerURIKey: "bad"},
	))
	_, err = overlay.Call(badCtx, &ptypes.Message{Payload: []byte(``)})
	require.EqualError(t, err, "handler failed to process: rpc is not supported")

	ctx = metadata.NewIncomingContext(context.Background(), metadata.Pairs(headerURIKey, "test"))
	overlay.context = fake.NewBadContext()
	_, err = overlay.Call(ctx, &ptypes.Message{Payload: []byte(``)})
	require.EqualError(t, err, "couldn't serialize result: fake error")
}

func TestOverlayServer_Stream(t *testing.T) {
	overlay := overlayServer{
		overlay: &overlay{
			router:      tree.NewRouter(3, AddressFactory{}),
			context:     json.NewContext(),
			addrFactory: AddressFactory{},
			me:          fake.NewAddress(0),
			closer:      &sync.WaitGroup{},
		},
		endpoints: make(map[string]*Endpoint),
	}

	overlay.endpoints["test"] = &Endpoint{Handler: testHandler{skip: true},
		streams: make(map[string]session.Session)}
	overlay.endpoints["bad"] = &Endpoint{Handler: testHandler{skip: true,
		err: xerrors.New("oops")}, streams: make(map[string]session.Session)}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	inCtx := metadata.NewIncomingContext(ctx, metadata.Pairs(
		headerURIKey, "test",
		headerStreamIDKey, "test"))

	err := overlay.Stream(newFakeServerStream(inCtx))
	require.NoError(t, err)

	stream := newFakeServerStream(inCtx)
	stream.err = xerrors.New("oops")
	err = overlay.Stream(stream)
	require.EqualError(t, err, "receive handshake: oops")

	overlay.router = fakeRouter{errFac: xerrors.New("oops")}
	err = overlay.Stream(newFakeServerStream(ctx))
	require.EqualError(t, err, "handshake: oops")

	overlay.router = fakeRouter{err: xerrors.New("oops")}
	err = overlay.Stream(newFakeServerStream(ctx))
	require.EqualError(t, err, "routing table: oops")

	overlay.router = tree.NewRouter(3, AddressFactory{})
	err = overlay.Stream(newFakeServerStream(ctx))
	require.EqualError(t, err, "handler '' is not registered")

	inCtx = metadata.NewIncomingContext(ctx, metadata.Pairs(headerURIKey, "unknown"))
	err = overlay.Stream(newFakeServerStream(inCtx))
	require.EqualError(t, err, "handler 'unknown' is not registered")

	inCtx = metadata.NewIncomingContext(ctx, metadata.Pairs(headerURIKey, "test"))
	err = overlay.Stream(newFakeServerStream(inCtx))
	require.EqualError(t, err, "failed to get streamID, result is empty")

	overlay.context = json.NewContext()
	overlay.router = tree.NewRouter(3, AddressFactory{})
	inCtx = metadata.NewIncomingContext(ctx, metadata.Pairs(headerURIKey, "bad", headerStreamIDKey, "test"))
	err = overlay.Stream(newFakeServerStream(inCtx))
	require.EqualError(t, err, "handler failed to process: oops")
}

func TestOverlay_Panic_GetCertificate(t *testing.T) {
	defer func() {
		r := recover()
		require.Equal(t, "certificate of the overlay must be populated", r)
	}()

	o := &overlay{
		certs: certs.NewInMemoryStore(),
	}

	o.GetCertificate()
}

func TestOverlay_Join(t *testing.T) {
	cert, err := makeCertificate()
	require.NoError(t, err)

	overlay := overlay{
		me:     fake.NewAddress(0),
		certs:  fakeCerts{},
		router: tree.NewRouter(3, AddressFactory{}),
		connMgr: fakeConnMgr{
			resp: ptypes.JoinResponse{Peers: []*ptypes.Certificate{{Value: cert.Leaf.Raw}}},
		},
		addrFactory: AddressFactory{},
	}

	err = overlay.Join("", "", nil)
	require.NoError(t, err)

	overlay.addrFactory = fake.AddressFactory{}
	err = overlay.Join("", "", nil)
	require.EqualError(t, err, "invalid address type 'fake.Address'")

	overlay.addrFactory = AddressFactory{}
	overlay.me = fake.NewBadAddress()
	err = overlay.Join("", "", nil)
	require.EqualError(t, err, "couldn't marshal own address: fake error")

	overlay.me = fake.NewAddress(0)
	overlay.certs = fakeCerts{err: xerrors.New("oops")}
	err = overlay.Join("", "", nil)
	require.EqualError(t, err, "couldn't fetch distant certificate: oops")

	overlay.certs = fakeCerts{}
	overlay.connMgr = fakeConnMgr{err: xerrors.New("oops")}
	err = overlay.Join("", "", nil)
	require.EqualError(t, err, "couldn't open connection: oops")

	overlay.connMgr = fakeConnMgr{resp: ptypes.JoinResponse{}, errConn: xerrors.New("oops")}
	err = overlay.Join("", "", nil)
	require.EqualError(t, err, "couldn't call join: oops")

	overlay.connMgr = fakeConnMgr{resp: ptypes.JoinResponse{Peers: []*ptypes.Certificate{{}}}}
	err = overlay.Join("", "", nil)
	require.EqualError(t, err,
		"couldn't parse certificate: asn1: syntax error: sequence truncated")
}

func TestConnManager_Acquire(t *testing.T) {
	dst, err := NewMinogrpc("127.0.0.1", 3334, nil)
	require.NoError(t, err)

	defer dst.GracefulClose()

	mgr := newConnManager(fake.NewAddress(0), certs.NewInMemoryStore())

	mgr.certs.Store(mgr.me, &tls.Certificate{})
	mgr.certs.Store(dst.GetAddress(), dst.GetCertificate())

	conn, err := mgr.Acquire(dst.GetAddress())
	require.NoError(t, err)
	require.NotNil(t, conn)

	_, err = mgr.Acquire(dst.GetAddress())
	require.NoError(t, err)
	require.Len(t, mgr.conns, 1)
	require.Equal(t, 2, mgr.counters[dst.GetAddress()])

	mgr.Release(dst.GetAddress())
	mgr.Release(dst.GetAddress())
	require.Len(t, mgr.conns, 0)
	require.Equal(t, 0, mgr.counters[dst.GetAddress()])

	_, err = mgr.Acquire(fake.NewAddress(1))
	require.EqualError(t, err, "certificate for 'fake.Address[1]' not found")

	mgr.conns = make(map[mino.Address]*grpc.ClientConn)
	mgr.certs.Delete(mgr.me)
	_, err = mgr.Acquire(dst.GetAddress())
	require.EqualError(t, err, "couldn't find server 'fake.Address[0]' certificate")

	mgr.certs.Store(mgr.me, dst.GetCertificate())
	_, err = mgr.Acquire(mgr.me)
	require.EqualError(t, err, "invalid address type 'fake.Address'")
}

// -----------------------------------------------------------------------------
// Utility functions

func makeInstances(t *testing.T, n int, call *fake.Call) ([]mino.Mino, []mino.RPC) {
	mm := make([]mino.Mino, n)
	rpcs := make([]mino.RPC, n)
	for i := range mm {
		m, err := NewMinogrpc("127.0.0.1", 3000+uint16(i), tree.NewRouter(2, AddressFactory{}))
		require.NoError(t, err)

		rpc, err := m.MakeRPC("test", testHandler{call: call}, fake.MessageFactory{})
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

type testHandler struct {
	mino.UnsupportedHandler
	call *fake.Call
	skip bool
	err  error
}

// Stream implements mino.Handler. It implements a simple receiver that will
// return the message received and close.
func (h testHandler) Stream(out mino.Sender, in mino.Receiver) error {
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

func (h testHandler) Process(req mino.Request) (serde.Message, error) {
	h.call.Add(req)

	return req.Message, nil
}

type emptyHandler struct {
	mino.UnsupportedHandler
}

func (h emptyHandler) Process(req mino.Request) (serde.Message, error) {
	return nil, nil
}

type fakeServerStream struct {
	grpc.ServerStream
	ch  chan *ptypes.Packet
	ctx context.Context
	err error
}

func newFakeServerStream(ctx context.Context) fakeServerStream {
	ch := make(chan *ptypes.Packet, 1)
	ch <- &ptypes.Packet{Serialized: []byte(`{}`)}

	return fakeServerStream{
		ch:  ch,
		ctx: ctx,
	}
}

func (s fakeServerStream) Context() context.Context {
	return s.ctx
}

func (s fakeServerStream) Send(m *ptypes.Packet) error {
	s.ch <- m
	return nil
}

func (s fakeServerStream) Recv() (*ptypes.Packet, error) {
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

type fakeRouter struct {
	router.Router

	errFac error
	err    error
}

func (r fakeRouter) GetHandshakeFactory() router.HandshakeFactory {
	return fakeHandshakeFac{err: r.errFac}
}

func (r fakeRouter) New(mino.Players) (router.RoutingTable, error) {
	return nil, r.err
}

func (r fakeRouter) TableOf(router.Handshake) (router.RoutingTable, error) {
	return nil, r.err
}

type fakeHandshakeFac struct {
	router.HandshakeFactory

	err error
}

func (fac fakeHandshakeFac) HandshakeOf(serde.Context, []byte) (router.Handshake, error) {
	return nil, fac.err
}
