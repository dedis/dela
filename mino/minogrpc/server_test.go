package minogrpc

import (
	"context"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/internal/tracing"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc/certs"
	"go.dedis.ch/dela/mino/minogrpc/ptypes"
	"go.dedis.ch/dela/mino/minogrpc/session"
	"go.dedis.ch/dela/mino/minogrpc/tokens"
	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/mino/router/tree"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
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
		require.True(t, to.Equal(from))
		require.IsType(t, fake.Message{}, msg)
	}

	// Start the shutdown procedure.
	cancel()

	for _, m := range mm {
		// This makes sure that the relay handlers have been closed by the
		// context.
		require.NoError(t, m.(*Minogrpc).GracefulStop())
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
					require.NoError(t, m.(*Minogrpc).GracefulStop())
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
		require.NoError(t, srvs[1].(*Minogrpc).GracefulStop())
		require.NoError(t, srvs[3].(*Minogrpc).GracefulStop())
		for _, srv := range srvs[5:] {
			require.NoError(t, srv.(*Minogrpc).GracefulStop())
		}
	}()

	// Shutdown one of the instance
	require.NoError(t, srvs[0].(*Minogrpc).GracefulStop())

	authority := fake.NewAuthorityFromMino(fake.NewSigner, srvs...)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sender, recvr, err := rpcs[1].Stream(ctx, authority)
	require.NoError(t, err)

	// Send a message to the shutted down instance to setup the relay, so that
	// we can try it will remove it and use another address later.
	err = <-sender.Send(fake.Message{}, srvs[0].GetAddress())
	checkError(t, err, srvs[0])

	// Test if the router learnt about the dead node and fixed the relay, while
	// opening relay to known nodes for the following test.
	iter := authority.Take(mino.ListFilter([]int{1, 4, 8, 11, 12, 13})).AddressIterator()
	for iter.HasNext() {
		to := iter.GetNext()
		errs := sender.Send(fake.Message{}, srvs[0].GetAddress(), to)
		checkError(t, <-errs, srvs[0])
		require.NoError(t, <-errs)

		from, _, err := recvr.Recv(context.Background())
		require.NoError(t, err)
		require.True(t, to.Equal(from))
	}

	// This node is a relay for sure by using the tree router, so we close it to
	// make sure the protocol can progress.
	srvs[4].(*Minogrpc).Stop()
	// Close also a leaf to see if we get the feedback that it has failed.
	srvs[2].(*Minogrpc).Stop()

	closed := []mino.Address{
		srvs[0].GetAddress(),
		srvs[4].GetAddress(),
		srvs[2].GetAddress(),
	}

	// Test if the network can progress with the loss of a relay.
	iter = authority.Take(mino.ListFilter([]int{3, 5, 6, 7, 9})).AddressIterator()
	for iter.HasNext() {
		to := iter.GetNext()
		errs := sender.Send(fake.Message{}, append([]mino.Address{to}, closed...)...)
		checkError(t, <-errs, srvs[0], srvs[2], srvs[4])
		checkError(t, <-errs, srvs[0], srvs[2], srvs[4])
		checkError(t, <-errs, srvs[0], srvs[2], srvs[4])
		require.NoError(t, <-errs)

		from, _, err := recvr.Recv(context.Background())
		require.NoError(t, err)
		require.True(t, to.Equal(from))
	}

	cancel()
}

func TestOverlayServer_Join(t *testing.T) {
	o, err := newOverlay(minoTemplate{
		myAddr: session.NewAddress("127.0.0.1:0"),
		certs:  certs.NewInMemoryStore(),
		router: tree.NewRouter(addressFac),
		curve:  elliptic.P521(),
		random: rand.Reader,
	})
	require.NoError(t, err)

	o.connMgr = fakeConnMgr{}

	overlay := &overlayServer{overlay: o}

	cert := overlay.GetCertificate()
	token := overlay.tokens.Generate(time.Hour)

	ctx := context.Background()
	req := &ptypes.JoinRequest{
		Token: token,
		Certificate: &ptypes.Certificate{
			Address: []byte{},
			Value:   cert.Leaf.Raw,
		},
	}

	resp, err := overlay.Join(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestOverlayJoin_InvalidToken_Join(t *testing.T) {
	overlay := overlayServer{
		overlay: &overlay{
			tokens: tokens.NewInMemoryHolder(),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := &ptypes.JoinRequest{Token: "abc"}

	_, err := overlay.Join(ctx, req)
	require.EqualError(t, err, "token 'abc' is invalid")
}

func TestOverlayJoin_BadAddress_Join(t *testing.T) {
	overlay := overlayServer{
		overlay: &overlay{
			tokens: tokens.NewInMemoryHolder(),
			certs:  certs.NewInMemoryStore(),
		},
	}

	overlay.certs.Store(fake.NewBadAddress(), fake.MakeCertificate(t, 0))

	token := overlay.tokens.Generate(time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := &ptypes.JoinRequest{Token: token}

	_, err := overlay.Join(ctx, req)
	require.EqualError(t, err, fake.Err("couldn't marshal address"))
}

func TestOverlayJoin_BadNetwork_Join(t *testing.T) {
	overlay := overlayServer{
		overlay: &overlay{
			tokens:  tokens.NewInMemoryHolder(),
			certs:   certs.NewInMemoryStore(),
			connMgr: fakeConnMgr{err: fake.GetError()},
		},
	}

	overlay.certs.Store(session.NewAddress(""), fake.MakeCertificate(t, 0))

	token := overlay.tokens.Generate(time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := &ptypes.JoinRequest{Token: token}

	_, err := overlay.Join(ctx, req)
	require.EqualError(t, err,
		fake.Err("failed to share certificate: couldn't open connection"))
}

func TestOverlayJoin_BadConn_Join(t *testing.T) {
	overlay := overlayServer{
		overlay: &overlay{
			tokens:  tokens.NewInMemoryHolder(),
			certs:   certs.NewInMemoryStore(),
			connMgr: fakeConnMgr{errConn: fake.GetError()},
		},
	}

	overlay.certs.Store(session.NewAddress(""), fake.MakeCertificate(t, 0))

	token := overlay.tokens.Generate(time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := &ptypes.JoinRequest{Token: token}

	_, err := overlay.Join(ctx, req)
	require.EqualError(t, err, fake.Err("failed to share certificate: couldn't call share"))
}

func TestOverlayServer_Share(t *testing.T) {
	overlay := overlayServer{
		overlay: &overlay{
			certs:       certs.NewInMemoryStore(),
			router:      tree.NewRouter(addressFac),
			addrFactory: addressFac,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	from := session.NewAddress("127.0.0.1:8080")
	fromBuf, err := from.MarshalText()
	require.NoError(t, err)

	cert := fake.MakeCertificate(t, 1, net.IPv4(127, 0, 0, 1))

	req := &ptypes.Certificate{
		Address: fromBuf,
		Value:   cert.Leaf.Raw,
	}

	resp, err := overlay.Share(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	shared, err := overlay.certs.Load(from)
	require.NoError(t, err)
	require.NotNil(t, shared)

	_, err = overlay.Share(ctx, &ptypes.Certificate{})
	require.EqualError(t, err,
		"couldn't parse certificate: x509: malformed certificate")
}

func TestOverlayServer_Call(t *testing.T) {
	overlay := overlayServer{
		overlay: &overlay{
			router:      tree.NewRouter(addressFac),
			context:     json.NewContext(),
			addrFactory: addressFac,
		},
		endpoints: map[string]*Endpoint{
			"test":  {Handler: testHandler{}, Factory: fake.MessageFactory{}},
			"empty": {Handler: emptyHandler{}, Factory: fake.MessageFactory{}},
		},
	}

	ctx := makeCtx(headerURIKey, "test")

	resp, err := overlay.Call(ctx, &ptypes.Message{Payload: []byte(`{}`)})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, []byte(`{}`), resp.GetPayload())

	ctx = makeCtx(headerURIKey, "empty")

	resp, err = overlay.Call(ctx, &ptypes.Message{Payload: []byte(`{}`)})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Nil(t, resp.GetPayload())
}

func TestOverlayServer_UnknownHandler_Call(t *testing.T) {
	overlay := overlayServer{
		endpoints: make(map[string]*Endpoint),
	}

	ctx := makeCtx(headerURIKey, "unknown")

	_, err := overlay.Call(ctx, nil)
	require.EqualError(t, err, "handler 'unknown' is not registered")

	_, err = overlay.Call(context.Background(), nil)
	require.EqualError(t, err, "handler '' is not registered")

	_, err = overlay.Call(makeCtx(), nil)
	require.EqualError(t, err, "handler '' is not registered")
}

func TestOverlayServer_BadHandlerFactory_Call(t *testing.T) {
	overlay := overlayServer{
		overlay: &overlay{},
		endpoints: map[string]*Endpoint{
			"test": {Handler: testHandler{}, Factory: fake.NewBadMessageFactory()},
		},
	}

	ctx := makeCtx(headerURIKey, "test")

	_, err := overlay.Call(ctx, &ptypes.Message{Payload: []byte(``)})
	require.EqualError(t, err, fake.Err("couldn't deserialize message"))
}

func TestOverlayServer_BadHandler_Call(t *testing.T) {
	overlay := overlayServer{
		overlay: &overlay{
			addrFactory: addressFac,
		},
		endpoints: map[string]*Endpoint{
			"test": {Handler: mino.UnsupportedHandler{}, Factory: fake.MessageFactory{}},
		},
	}

	ctx := makeCtx(headerURIKey, "test")

	_, err := overlay.Call(ctx, &ptypes.Message{Payload: []byte(``)})
	require.EqualError(t, err, "handler failed to process: rpc is not supported")
}

func TestOverlayServer_BadResponseFactory_Call(t *testing.T) {
	overlay := overlayServer{
		overlay: &overlay{
			addrFactory: addressFac,
			context:     fake.NewBadContext(),
		},
		endpoints: map[string]*Endpoint{
			"test": {Handler: testHandler{}, Factory: fake.MessageFactory{}},
		},
	}

	ctx := makeCtx(headerURIKey, "test")

	_, err := overlay.Call(ctx, &ptypes.Message{Payload: []byte(``)})
	require.EqualError(t, err, fake.Err("couldn't serialize result"))
}

func TestOverlayServer_Stream(t *testing.T) {
	overlay := overlayServer{
		overlay: &overlay{
			router:      tree.NewRouter(addressFac),
			context:     json.NewContext(),
			addrFactory: addressFac,
			myAddr:      session.NewAddress("127.0.0.1:0"),
			closer:      &sync.WaitGroup{},
			connMgr:     fakeConnMgr{},
		},
		endpoints: map[string]*Endpoint{
			"test": {
				Handler: testHandler{skip: true},
				streams: make(map[string]session.Session),
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	inCtx := metadata.NewIncomingContext(ctx, metadata.Pairs(
		headerURIKey, "test",
		headerStreamIDKey, "streamTest",
		session.HandshakeKey, "{}"))

	wg := sync.WaitGroup{}
	wg.Add(5)
	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()

			err := overlay.Stream(&fakeSrvStream{ctx: inCtx})
			require.NoError(t, err)
		}()
	}
	wg.Wait()
	overlay.closer.Wait()
	require.Empty(t, overlay.endpoints["test"].streams)

	overlay.endpoints["test"].streams["streamTest"] = fakeSession{numParents: 1}
	err := overlay.Stream(&fakeSrvStream{ctx: inCtx})
	overlay.closer.Wait()
	require.NoError(t, err)
	require.Len(t, overlay.endpoints["test"].streams, 1)
}

func TestOverlay_MissingHeaders_Stream(t *testing.T) {
	overlay := overlayServer{
		overlay: &overlay{
			closer: &sync.WaitGroup{},
		},
	}

	stream := &fakeSrvStream{ctx: context.Background()}

	err := overlay.Stream(stream)
	require.EqualError(t, err, "missing headers")
}

func TestOverlay_MalformedRtingTable_Stream(t *testing.T) {
	overlay := overlayServer{
		overlay: &overlay{
			closer:      &sync.WaitGroup{},
			router:      badRouter{},
			addrFactory: addressFac,
		},
	}

	ctx := makeCtx(headerStreamIDKey, "abc", headerAddressKey, "{}")

	stream := &fakeSrvStream{ctx: ctx}

	err := overlay.Stream(stream)
	require.EqualError(t, err, fake.Err("routing table: failed to create"))

	stream.ctx = makeCtx(
		headerStreamIDKey, "abc",
		headerAddressKey, "{}",
		session.HandshakeKey, "{}")

	err = overlay.Stream(stream)
	require.EqualError(t, err, fake.Err("routing table: invalid handshake"))

	overlay.router = badRouter{errFac: true}
	err = overlay.Stream(stream)
	require.EqualError(t, err, fake.Err("routing table: malformed handshake"))
}

func TestOverlay_UnknownHandler_Stream(t *testing.T) {
	overlay := overlayServer{
		overlay: &overlay{
			closer:      &sync.WaitGroup{},
			router:      tree.NewRouter(addressFac),
			addrFactory: addressFac,
			context:     json.NewContext(),
		},
	}

	ctx := makeCtx(headerStreamIDKey, "abc", session.HandshakeKey, "{}")

	stream := &fakeSrvStream{ctx: ctx}

	err := overlay.Stream(stream)
	require.EqualError(t, err, "handler '' is not registered")

	stream.ctx = makeCtx(
		headerURIKey, "unknown",
		session.HandshakeKey, "{}",
		headerStreamIDKey, "abc")

	err = overlay.Stream(stream)
	require.EqualError(t, err, "handler 'unknown' is not registered")
}

func TestOverlay_BadStreamID_Stream(t *testing.T) {
	overlay := overlayServer{
		overlay: &overlay{
			closer:      &sync.WaitGroup{},
			router:      tree.NewRouter(addressFac),
			addrFactory: addressFac,
			context:     json.NewContext(),
		},
	}

	ctx := makeCtx(headerStreamIDKey, "", session.HandshakeKey, "{}")

	stream := &fakeSrvStream{ctx: ctx}

	err := overlay.Stream(stream)
	require.EqualError(t, err, "unexpected empty stream ID")
}

func TestOverlay_BadHandler_Stream(t *testing.T) {
	overlay := overlayServer{
		overlay: &overlay{
			myAddr:      session.NewAddress(""),
			closer:      &sync.WaitGroup{},
			router:      tree.NewRouter(addressFac),
			addrFactory: addressFac,
			context:     json.NewContext(),
			connMgr:     fakeConnMgr{},
		},
		endpoints: map[string]*Endpoint{
			"test": {
				Handler: testHandler{skip: true, err: fake.GetError()},
				streams: make(map[string]session.Session),
			},
		},
	}

	ctx := makeCtx(headerURIKey, "test", headerStreamIDKey, "abc", session.HandshakeKey, "{}")

	stream := &fakeSrvStream{ctx: ctx}

	err := overlay.Stream(stream)
	require.EqualError(t, err, fake.Err("handler failed to process"))
}

func TestOverlay_BadConn_Stream(t *testing.T) {
	overlay := overlayServer{
		overlay: &overlay{
			myAddr:      session.NewAddress(""),
			closer:      &sync.WaitGroup{},
			router:      tree.NewRouter(addressFac),
			addrFactory: addressFac,
			context:     json.NewContext(),
			connMgr:     fakeConnMgr{},
		},
		endpoints: map[string]*Endpoint{
			"test": {
				Handler: testHandler{},
				streams: make(map[string]session.Session),
			},
		},
	}

	ctx := makeCtx(headerURIKey, "test", headerStreamIDKey, "abc", session.HandshakeKey, "{}")

	stream := &fakeSrvStream{ctx: ctx, err: fake.GetError()}

	err := overlay.Stream(stream)
	require.EqualError(t, err, fake.Err("failed to send header"))
}

func TestOverlay_BadParentGateway_Stream(t *testing.T) {
	overlay := overlayServer{
		overlay: &overlay{
			myAddr:      session.NewAddress(""),
			closer:      &sync.WaitGroup{},
			router:      tree.NewRouter(addressFac),
			addrFactory: addressFac,
			context:     json.NewContext(),
			connMgr:     fakeConnMgr{err: fake.GetError()},
		},
		endpoints: map[string]*Endpoint{
			"test": {
				Handler: testHandler{},
				streams: make(map[string]session.Session),
			},
		},
	}

	ctx := makeCtx(headerURIKey, "test", headerStreamIDKey, "abc", session.HandshakeKey, "{}")

	stream := &fakeSrvStream{ctx: ctx, err: fake.GetError()}

	err := overlay.Stream(stream)
	require.EqualError(t, err, fake.Err("gateway connection failed"))
}

func TestOverlay_Forward(t *testing.T) {
	overlay := overlayServer{
		overlay: &overlay{
			router:      tree.NewRouter(addressFac),
			context:     json.NewContext(),
			addrFactory: addressFac,
			myAddr:      session.NewAddress("127.0.0.1:0"),
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
	o, err := newOverlay(minoTemplate{
		myAddr: session.NewAddress("127.0.0.1:0"),
		certs:  certs.NewInMemoryStore(),
		curve:  elliptic.P521(),
		random: rand.Reader,
	})
	require.NoError(t, err)

	cert, err := o.certs.Load(session.NewAddress("127.0.0.1:0"))
	require.NoError(t, err)
	require.NotNil(t, cert)
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

func TestOverlay_Panic2_GetCertificate(t *testing.T) {
	defer func() {
		r := recover()
		require.EqualError(t, r.(error), fake.Err("certificate of the overlay is inaccessible"))
	}()

	o := &overlay{
		certs: fakeCerts{errLoad: fake.GetError()},
	}

	o.GetCertificate()
}

func TestOverlay_Join(t *testing.T) {
	overlay, err := newOverlay(minoTemplate{
		myAddr: session.NewAddress("127.0.0.1:0"),
		certs:  certs.NewInMemoryStore(),
		router: tree.NewRouter(addressFac),
		fac:    addressFac,
		curve:  elliptic.P521(),
		random: rand.Reader,
	})
	require.NoError(t, err)

	overlay.connMgr = fakeConnMgr{
		resp: ptypes.JoinResponse{
			Peers: []*ptypes.Certificate{{Value: overlay.GetCertificate().Leaf.Raw}},
		},
	}

	overlay.certs = fakeCerts{}
	err = overlay.Join("", "", nil)
	require.NoError(t, err)

	overlay.myAddr = session.NewAddress("127.0.0.1:0")
	overlay.certs = fakeCerts{err: fake.GetError()}
	err = overlay.Join("", "", nil)
	require.EqualError(t, err, fake.Err("couldn't fetch distant certificate"))

	overlay.certs = fakeCerts{}
	overlay.connMgr = fakeConnMgr{err: fake.GetError()}
	err = overlay.Join("", "", nil)
	require.EqualError(t, err, fake.Err("couldn't open connection"))

	overlay.connMgr = fakeConnMgr{resp: ptypes.JoinResponse{}, errConn: fake.GetError()}
	err = overlay.Join("", "", nil)
	require.EqualError(t, err, fake.Err("couldn't call join"))

	overlay.connMgr = fakeConnMgr{resp: ptypes.JoinResponse{Peers: []*ptypes.Certificate{{}}}}
	err = overlay.Join("", "", nil)
	require.EqualError(t, err,
		"couldn't parse certificate: x509: malformed certificate")
}

func TestConnManager_Acquire(t *testing.T) {
	addr := ParseAddress("127.0.0.1", 0)

	dst, err := NewMinogrpc(addr, nil)
	require.NoError(t, err)

	defer dst.GracefulStop()

	mgr := newConnManager(fake.NewAddress(0), certs.NewInMemoryStore())

	certs := mgr.certs
	certs.Store(mgr.myAddr, &tls.Certificate{})
	certs.Store(dst.GetAddress(), dst.GetCertificate())

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
}

func TestConnManager_FailLoadDistantCert_Acquire(t *testing.T) {
	mgr := newConnManager(fake.NewAddress(0), certs.NewInMemoryStore())
	mgr.certs = fakeCerts{errLoad: fake.GetError()}

	_, err := mgr.Acquire(fake.NewAddress(0))
	require.EqualError(t, err, fake.Err("failed to retrieve transport credential: while loading distant cert"))
}

func TestConnManager_MissingCert_Acquire(t *testing.T) {
	mgr := newConnManager(fake.NewAddress(0), certs.NewInMemoryStore())

	_, err := mgr.Acquire(fake.NewAddress(1))
	require.EqualError(t, err, "failed to retrieve transport credential: certificate for 'fake.Address[1]' not found")
}

func TestConnManager_FailLoadOwnCert_Acquire(t *testing.T) {
	mgr := newConnManager(fake.NewAddress(0), certs.NewInMemoryStore())
	mgr.certs = fakeCerts{
		errLoad: fake.GetError(),
		counter: fake.NewCounter(1),
	}

	_, err := mgr.Acquire(fake.NewAddress(0))
	require.EqualError(t, err, fake.Err("failed to retrieve transport credential: while loading own cert"))
}

func TestConnManager_MissingOwnCert_Acquire(t *testing.T) {
	mgr := newConnManager(fake.NewAddress(0), certs.NewInMemoryStore())
	mgr.certs.Store(fake.NewAddress(1), fake.MakeCertificate(t, 0))

	_, err := mgr.Acquire(fake.NewAddress(1))
	require.EqualError(t, err, "failed to retrieve transport credential: couldn't find server 'fake.Address[0]' certificate")
}

func TestConnManager_BadAddress_Acquire(t *testing.T) {
	mgr := newConnManager(fake.NewAddress(0), certs.NewInMemoryStore())
	mgr.certs.Store(fake.NewAddress(0), fake.MakeCertificate(t, 0))

	_, err := mgr.Acquire(mgr.myAddr)
	require.EqualError(t, err, "invalid address type 'fake.Address'")
}

func TestConnManager_BadTracer_Acquire(t *testing.T) {
	addr := ParseAddress("127.0.0.1", 0)

	dst, err := NewMinogrpc(addr, nil)
	require.NoError(t, err)

	defer dst.GracefulStop()

	mgr := newConnManager(fake.NewAddress(0), certs.NewInMemoryStore())

	getTracerForAddr = fake.GetTracerForAddrWithError

	certs := mgr.certs
	certs.Store(mgr.myAddr, &tls.Certificate{})
	certs.Store(dst.GetAddress(), dst.GetCertificate())

	dstAddr := dst.GetAddress()
	_, err = mgr.Acquire(dstAddr)
	require.EqualError(
		t,
		err,
		fmt.Sprintf("failed to get tracer for addr %s: %s", dst.GetAddress(), fake.GetError().Error()),
	)

	getTracerForAddr = tracing.GetTracerForAddr
}

// -----------------------------------------------------------------------------
// Utility functions

func makeInstances(t *testing.T, n int, call *fake.Call) ([]mino.Mino, []mino.RPC) {
	mm := make([]mino.Mino, n)
	rpcs := make([]mino.RPC, n)
	for i := range mm {
		addr := ParseAddress("127.0.0.1", 0)

		m, err := NewMinogrpc(addr, tree.NewRouter(addressFac))
		require.NoError(t, err)

		mm[i] = m
		rpcs[i] = mino.MustCreateRPC(m, "test", testHandler{call: call}, fake.MessageFactory{})

		for _, k := range mm[:i] {
			km := k.(*Minogrpc)

			m.GetCertificateStore().Store(k.GetAddress(), km.GetCertificate())
			km.GetCertificateStore().Store(m.GetAddress(), m.GetCertificate())
		}
	}

	return mm, rpcs
}

func checkError(t *testing.T, err error, mm ...mino.Mino) {
	require.Error(t, err)

	for _, m := range mm {
		if err.Error() == fmt.Sprintf("no route to %s: address is unreachable", m.GetAddress()) {
			return
		}
	}

	t.Fatal("unexpected error", err)
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

type fakeCerts struct {
	certs.Storage
	err      error
	errLoad  error
	errStore error
	counter  *fake.Counter
}

func (s fakeCerts) Store(mino.Address, *tls.Certificate) error {
	return s.errStore
}

func (s fakeCerts) Load(mino.Address) (*tls.Certificate, error) {
	if s.errStore != nil {
		return nil, s.errLoad
	}

	if s.errLoad != nil && s.counter.Done() {
		return nil, s.errLoad
	}

	s.counter.Decrease()

	return &tls.Certificate{Leaf: &x509.Certificate{Raw: []byte{0x89}}}, nil
}

func (s fakeCerts) Fetch(certs.Dialable, []byte) error {
	return s.err
}

type fakeSession struct {
	session.Session

	numParents int
}

func (sess fakeSession) GetNumParents() int {
	return sess.numParents
}

func (fakeSession) Listen(p session.Relay, t router.RoutingTable, c chan struct{}) {
	close(c)
}

func (fakeSession) RecvPacket(mino.Address, *ptypes.Packet) (*ptypes.Ack, error) {
	return &ptypes.Ack{}, nil
}
