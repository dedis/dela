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
	"go.dedis.ch/dela/mino/router/tree"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestIntegration_Scenario_Stream(t *testing.T) {

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

func TestIntegration_Scenario_Call(t *testing.T) {
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

func TestMinogrpc_Scenario_Failures(t *testing.T) {
	srvs, rpcs := makeInstances(t, 14, nil)
	defer func() {
		require.NoError(t, srvs[1].(*Minogrpc).GracefulClose())
		require.NoError(t, srvs[3].(*Minogrpc).GracefulClose())
		for _, srv := range srvs[5:] {
			require.NoError(t, srv.(*Minogrpc).GracefulClose())
		}
	}()

	// Shutdown one of the instance
	require.NoError(t, srvs[0].(*Minogrpc).GracefulClose())

	authority := fake.NewAuthorityFromMino(fake.NewSigner, srvs...)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sender, recvr, err := rpcs[1].Stream(ctx, authority)
	require.NoError(t, err)

	// Send a message to the shutted down instance to setup the relay, so that
	// we can try it will remove it and use another address later.
	err = <-sender.Send(fake.Message{}, srvs[0].GetAddress())
	require.EqualError(t, err, "no route to 127.0.0.1:3000: address is unreachable")

	// Test if the router learnt about the dead node and fixed the relay, while
	// opening relay to known nodes for the following test.
	iter := authority.Take(mino.ListFilter([]int{1, 4, 8, 11, 12, 13})).AddressIterator()
	for iter.HasNext() {
		to := iter.GetNext()
		errs := sender.Send(fake.Message{}, srvs[0].GetAddress(), to)
		require.EqualError(t, <-errs, "no route to 127.0.0.1:3000: address is unreachable")
		require.NoError(t, <-errs)

		from, _, err := recvr.Recv(context.Background())
		require.NoError(t, err)
		require.Equal(t, to, from)
	}

	// This node is a relay for sure by using the tree router, so we close it to
	// make sure the protocol can progress.
	srvs[4].(*Minogrpc).Close()
	// Close also a leaf to see if we get the feedback that it has failed.
	srvs[2].(*Minogrpc).Close()

	closed := []mino.Address{
		srvs[0].GetAddress(),
		srvs[4].GetAddress(),
		srvs[2].GetAddress(),
	}

	re := `^no route to 127\.0\.0\.1:300[042]: address is unreachable$`

	// Test if the network can progress with the lost of a relay.
	iter = authority.Take(mino.ListFilter([]int{3, 5, 6, 7, 9})).AddressIterator()
	for iter.HasNext() {
		to := iter.GetNext()
		errs := sender.Send(fake.Message{}, append([]mino.Address{to}, closed...)...)
		require.Regexp(t, re, <-errs)
		require.Regexp(t, re, <-errs)
		require.Regexp(t, re, <-errs)
		require.NoError(t, <-errs)

		from, _, err := recvr.Recv(context.Background())
		require.NoError(t, err)
		require.Equal(t, to, from)
	}

	cancel()
}

func TestOverlayServer_Join(t *testing.T) {
	overlay := overlayServer{
		overlay: &overlay{
			tokens:  fakeTokens{},
			certs:   certs.NewInMemoryStore(),
			router:  tree.NewRouter(AddressFactory{}),
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
			router:      tree.NewRouter(AddressFactory{}),
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
			router:      tree.NewRouter(AddressFactory{}),
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

	badCtx := makeCtx(headerURIKey, "unknown")
	_, err = overlay.Call(badCtx, nil)
	require.EqualError(t, err, "handler 'unknown' is not registered")

	_, err = overlay.Call(context.Background(), nil)
	require.EqualError(t, err, "handler '' is not registered")

	_, err = overlay.Call(makeCtx(), nil)
	require.EqualError(t, err, "handler '' is not registered")

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
			router:      tree.NewRouter(AddressFactory{}),
			context:     json.NewContext(),
			addrFactory: AddressFactory{},
			me:          fake.NewAddress(0),
			closer:      &sync.WaitGroup{},
			connMgr:     fakeConnMgr{},
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
		headerStreamIDKey, "test",
		session.HandshakeKey, "{}"))

	err := overlay.Stream(&fakeSrvStream{ctx: inCtx})
	require.NoError(t, err)

	err = overlay.Stream(&fakeSrvStream{ctx: ctx})
	require.EqualError(t, err, "missing headers")

	err = overlay.Stream(&fakeSrvStream{ctx: makeCtx()})
	require.EqualError(t, err, "routing table: headers are empty")

	overlay.router = badRouter{}
	badCtx := makeCtx(headerStreamIDKey, "abc", headerAddressKey, "{}")
	err = overlay.Stream(&fakeSrvStream{ctx: badCtx})
	require.EqualError(t, err, "routing table: failed to create: oops")

	overlay.router = badRouter{errFac: true}
	err = overlay.Stream(&fakeSrvStream{ctx: inCtx})
	require.EqualError(t, err, "routing table: malformed handshake: oops")

	overlay.router = badRouter{}
	err = overlay.Stream(&fakeSrvStream{ctx: inCtx})
	require.EqualError(t, err, "routing table: invalid handshake: oops")

	overlay.router = tree.NewRouter(AddressFactory{})
	badCtx = makeCtx(session.HandshakeKey, "{}", headerStreamIDKey, "abc")
	err = overlay.Stream(&fakeSrvStream{ctx: badCtx})
	require.EqualError(t, err, "handler '' is not registered")

	badCtx = makeCtx(session.HandshakeKey, "{}", headerURIKey, "unknown", headerStreamIDKey, "abc")
	err = overlay.Stream(&fakeSrvStream{ctx: badCtx})
	require.EqualError(t, err, "handler 'unknown' is not registered")

	badCtx = makeCtx(session.HandshakeKey, "{}", headerURIKey, "test")
	err = overlay.Stream(&fakeSrvStream{ctx: badCtx})
	require.EqualError(t, err, "unexpected empty stream ID")

	overlay.context = json.NewContext()
	overlay.router = tree.NewRouter(AddressFactory{})
	badCtx = makeCtx(headerURIKey, "bad", headerStreamIDKey, "test", session.HandshakeKey, "{}")
	err = overlay.Stream(&fakeSrvStream{ctx: badCtx})
	require.EqualError(t, err, "handler failed to process: oops")

	err = overlay.Stream(&fakeSrvStream{ctx: inCtx, err: xerrors.New("oops")})
	require.EqualError(t, err, "failed to send header: oops")

	overlay.connMgr = fakeConnMgr{err: xerrors.New("oops")}
	err = overlay.Stream(&fakeSrvStream{ctx: inCtx})
	require.EqualError(t, err, "gateway connection failed: oops")
}

func TestOverlay_Forward(t *testing.T) {
	overlay := overlayServer{
		overlay: &overlay{
			router:      tree.NewRouter(AddressFactory{}),
			context:     json.NewContext(),
			addrFactory: AddressFactory{},
			me:          fake.NewAddress(0),
			closer:      &sync.WaitGroup{},
			connMgr:     fakeConnMgr{},
		},
		endpoints: make(map[string]*Endpoint),
	}

	overlay.endpoints["test"] = &Endpoint{
		Handler: testHandler{skip: true},
		streams: map[string]session.Session{
			"stream-1": fakeSession{},
		},
	}

	ctx := makeCtx(headerURIKey, "test", headerStreamIDKey, "stream-1")

	ack, err := overlay.Forward(ctx, &ptypes.Packet{})
	require.NoError(t, err)
	require.NotNil(t, ack)

	_, err = overlay.Forward(context.Background(), &ptypes.Packet{})
	require.EqualError(t, err, "no header in the context")

	_, err = overlay.Forward(makeCtx(headerURIKey, "unknown"), &ptypes.Packet{})
	require.EqualError(t, err, "handler 'unknown' is not registered")

	_, err = overlay.Forward(makeCtx(headerURIKey, "test", headerStreamIDKey, "nope"), &ptypes.Packet{})
	require.EqualError(t, err, "no stream 'nope' found")
}

func TestOverlay_New(t *testing.T) {
	o, err := newOverlay(fake.NewAddress(0), nil, nil, fake.NewContext())
	require.NoError(t, err)
	require.NotNil(t, o.certs.Load(fake.NewAddress(0)))

	_, err = newOverlay(fake.NewBadAddress(), nil, nil, fake.NewContext())
	require.EqualError(t, err, "failed to marshal address: fake error")
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
		router: tree.NewRouter(AddressFactory{}),
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
		m, err := NewMinogrpc("127.0.0.1", 3000+uint16(i), tree.NewRouter(AddressFactory{}))
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

func makeCtx(kv ...string) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	return metadata.NewIncomingContext(ctx, metadata.Pairs(kv...))
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

type fakeSrvStream struct {
	ptypes.Overlay_StreamServer
	ctx context.Context
	err error
}

func (s fakeSrvStream) SendHeader(metadata.MD) error {
	return s.err
}

func (s fakeSrvStream) Context() context.Context {
	return s.ctx
}

func (s fakeSrvStream) Recv() (*ptypes.Packet, error) {
	return nil, s.err
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

type fakeSession struct {
	session.Session
}

func (fakeSession) RecvPacket(mino.Address, *ptypes.Packet) (*ptypes.Ack, error) {
	return &ptypes.Ack{}, nil
}
