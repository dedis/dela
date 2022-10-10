package minogrpc

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/internal/tracing"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc/certs"
	"go.dedis.ch/dela/mino/minogrpc/session"
	"go.dedis.ch/dela/mino/minogrpc/tokens"
	"go.dedis.ch/dela/mino/router/tree"
	"google.golang.org/grpc"
)

func TestMinogrpc_New(t *testing.T) {
	addr := ParseAddress("127.0.0.1", 3333)

	router := tree.NewRouter(addressFac)

	m, err := NewMinogrpc(addr, nil, router)
	require.NoError(t, err)

	require.Equal(t, "127.0.0.1:3333", m.GetAddress().String())
	require.Empty(t, m.segments)

	cert := m.GetCertificateChain()
	require.NotNil(t, cert)

	<-m.started
	require.NoError(t, m.GracefulStop())
}

func TestMinogrpc_New_FailedParsePublic(t *testing.T) {
	l := listener
	defer func() {
		listener = l
	}()

	listener = func(network, address string) (net.Listener, error) {
		return fakeListener{addr: ":xxx"}, nil
	}

	router := tree.NewRouter(addressFac)

	addr := net.IPAddr{}

	_, err := NewMinogrpc(&addr, nil, router)
	require.EqualError(t, err, "failed to parse public URL: parse \"//:xxx\": invalid port \":xxx\" after host")
}

func TestMinogrpc_FailGenerateKey_New(t *testing.T) {
	addr := ParseAddress("127.0.0.1", 3333)
	router := tree.NewRouter(addressFac)

	_, err := NewMinogrpc(addr, nil, router, WithRandom(badReader{}))
	require.EqualError(t, err, fake.Err("overlay: cert private key"))
}

func TestMinogrpc_FailCreateCertNew(t *testing.T) {
	addr := ParseAddress("127.0.0.1", 3333)
	router := tree.NewRouter(addressFac)

	_, err := NewMinogrpc(addr, nil, router, WithCertificateKey(struct{}{}, struct{}{}))
	require.Error(t, err)
	require.Contains(t, err.Error(), "overlay: certificate failed: while creating: x509: ")
}

func TestMinogrpc_FailStoreCerTNew(t *testing.T) {
	addr := ParseAddress("127.0.0.1", 3333)
	router := tree.NewRouter(addressFac)

	_, err := NewMinogrpc(addr, nil, router, WithStorage(fakeCerts{errStore: fake.GetError()}))
	require.EqualError(t, err, fake.Err("overlay: certificate failed: while storing"))
}

func TestMinogrpc_FailLoadCerTNew(t *testing.T) {
	addr := ParseAddress("127.0.0.1", 3333)
	router := tree.NewRouter(addressFac)

	_, err := NewMinogrpc(addr, nil, router, WithStorage(fakeCerts{errLoad: fake.GetError()}))
	require.EqualError(t, err, fake.Err("overlay: while loading cert"))
}

func TestMinogrpc_FailedBadCerTNew(t *testing.T) {
	addr := ParseAddress("127.0.0.1", 3333)
	router := tree.NewRouter(addressFac)

	_, err := NewMinogrpc(addr, nil, router, WithStorage(fakeCerts{}))
	require.EqualError(t, err, "failed to parse chain: x509: malformed certificate")
}

func TestMinogrpc_WithCerTNew(t *testing.T) {
	addr := ParseAddress("127.0.0.1", 3333)
	router := tree.NewRouter(addressFac)

	cert, _ := fake.MakeFullCertificate(t)

	m, err := NewMinogrpc(addr, nil, router, WithCert(cert))
	require.NoError(t, err)

	defer m.GracefulStop()

	chain := m.GetCertificateChain()
	require.Equal(t, certs.CertChain(cert.Certificate[0]), chain)
}

func TestMinogrpc_BadAddress_New(t *testing.T) {
	addr := ParseAddress("123.4.5.6", 1)
	router := tree.NewRouter(addressFac)

	_, err := NewMinogrpc(addr, nil, router)
	require.Error(t, err)
	// Funny enough, macos would output:
	//   couldn't start the server: failed to listen: listen tcp 123.4.5.6:1:
	//     bind: can't assign requested address
	// While linux outpus:
	//   couldn't start the server: failed to listen: listen tcp 123.4.5.6:1:
	//     bind: cannot assign requested address
	require.Regexp(t, "^failed to bind: listen tcp 123.4.5.6:1:", err)
}

func TestMinogrpc_BadTracer_New(t *testing.T) {
	getTracerForAddr = fake.GetTracerForAddrWithError

	addr := ParseAddress("127.0.0.1", 3333)

	router := tree.NewRouter(addressFac)

	_, err := NewMinogrpc(addr, nil, router)
	require.EqualError(t, err, fake.Err("failed to get tracer for addr 127.0.0.1:3333"))

	getTracerForAddr = tracing.GetTracerForAddr
}

func TestMinogrpc_GetTrafficWatcher(t *testing.T) {
	m := Minogrpc{}
	m.GetTrafficWatcher()
}

func TestMinogrpc_GetAddressFactory(t *testing.T) {
	m := &Minogrpc{}
	require.IsType(t, addressFac, m.GetAddressFactory())
}

func TestMinogrpc_GetAddress(t *testing.T) {
	addr := session.NewAddress("")
	minoGrpc := &Minogrpc{
		overlay: &overlay{myAddr: addr},
	}

	require.Equal(t, addr, minoGrpc.GetAddress())
}

func TestMinogrpc_Token(t *testing.T) {
	minoGrpc := &Minogrpc{
		overlay: &overlay{tokens: tokens.NewInMemoryHolder()},
	}

	token := minoGrpc.GenerateToken(time.Minute)
	require.True(t, minoGrpc.tokens.Verify(token))
}

func TestMinogrpc_GracefulClose(t *testing.T) {
	m := &Minogrpc{
		overlay: &overlay{
			closer:  new(sync.WaitGroup),
			connMgr: fakeConnMgr{},
		},
		server:  grpc.NewServer(),
		closing: make(chan error),
	}

	close(m.closing)
	err := m.GracefulStop()
	require.NoError(t, err)

	m.closing = make(chan error, 1)
	m.closing <- fake.GetError()
	err = m.GracefulStop()
	require.EqualError(t, err, fake.Err("server stopped unexpectedly"))

	m.closing = make(chan error)
	close(m.closing)
	m.connMgr = fakeConnMgr{len: 1}
	err = m.GracefulStop()
	require.EqualError(t, err, "connection manager not empty: 1")
}

func TestMinogrpc_WithSegment(t *testing.T) {
	m := &Minogrpc{}
	ns := "Test"

	newMino := m.WithSegment(ns)

	newMinoGrpc, ok := newMino.(*Minogrpc)
	require.True(t, ok)
	require.Equal(t, ns, newMinoGrpc.segments[0])

	newMino = m.WithSegment("")
	require.Equal(t, m, newMino)
}

func TestMinogrpc_CreateRPC(t *testing.T) {
	m := Minogrpc{
		overlay:   &overlay{},
		endpoints: make(map[string]*Endpoint),
	}

	mNs := m.WithSegment("segment")

	rpc, err := mNs.CreateRPC("name", emptyHandler{}, fake.MessageFactory{})
	require.NoError(t, err)

	expectedRPC := &RPC{
		factory: fake.MessageFactory{},
		overlay: &overlay{},
		uri:     "segment/name",
	}

	endpoint, ok := m.endpoints[expectedRPC.uri]
	require.True(t, ok)
	require.Equal(t, emptyHandler{}, endpoint.Handler)
	require.Equal(t, expectedRPC, rpc)

	_, err = mNs.CreateRPC("name", emptyHandler{}, fake.MessageFactory{})
	require.EqualError(t, err, "rpc 'segment/name' already exists")
}

func TestMinogrpc_InvalidSegment_CreateRPC(t *testing.T) {
	m := &Minogrpc{
		segments: []string{"example"},
	}

	handler := mino.UnsupportedHandler{}

	_, err := m.CreateRPC("/test", handler, fake.MessageFactory{})
	require.EqualError(t, err, "invalid segment in uri 'example//test': '/test'")

	_, err = m.CreateRPC(" test", handler, fake.MessageFactory{})
	require.EqualError(t, err, "invalid segment in uri 'example/ test': ' test'")

	_, err = m.CreateRPC("test$", handler, fake.MessageFactory{})
	require.EqualError(t, err, "invalid segment in uri 'example/test$': 'test$'")
}

func TestMinogrpc_String(t *testing.T) {
	minoGrpc := &Minogrpc{
		overlay: &overlay{myAddr: session.Address{}},
	}

	require.Equal(t, "mino[]", minoGrpc.String())
}

func TestMinogrpc_DecorateTrace_NoFound(t *testing.T) {
	ctx := context.Background()
	decorateServerTrace(ctx, nil, "", nil, nil, nil)
}

// -----------------------------------------------------------------------------
// Utility functions

type badReader struct{}

func (badReader) Read([]byte) (int, error) {
	return 0, fake.GetError()
}

type fakeListener struct {
	net.Listener
	addr string
}

func (l fakeListener) Addr() net.Addr {
	return fakeAddr{addr: l.addr}
}

type fakeAddr struct {
	net.Addr
	addr string
}

func (a fakeAddr) String() string {
	return a.addr
}
