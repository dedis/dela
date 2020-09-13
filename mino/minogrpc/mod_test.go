package minogrpc

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc/tokens"
	"go.dedis.ch/dela/mino/router/tree"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
)

func TestMinogrpc_New(t *testing.T) {
	addr := ParseAddress("127.0.0.1", 3333)

	m, err := NewMinogrpc(addr, tree.NewRouter(AddressFactory{}))
	require.NoError(t, err)

	require.Equal(t, "127.0.0.1:3333", m.GetAddress().String())
	require.Equal(t, "", m.namespace)

	cert := m.GetCertificate()
	require.NotNil(t, cert)

	require.NoError(t, m.GracefulStop())

	addr = ParseAddress("123.4.5.6", 1)

	_, err = NewMinogrpc(addr, tree.NewRouter(AddressFactory{}))
	require.Error(t, err)
	// Funny enough, macos would output:
	//   couldn't start the server: failed to listen: listen tcp 123.4.5.6:1:
	//     bind: can't assign requested address
	// While linux outpus:
	//   couldn't start the server: failed to listen: listen tcp 123.4.5.6:1:
	//     bind: cannot assign requested address
	require.Regexp(t, "^failed to bind: listen tcp 123.4.5.6:1:", err)
}

func TestMinogrpc_GetAddressFactory(t *testing.T) {
	m := &Minogrpc{}
	require.IsType(t, AddressFactory{}, m.GetAddressFactory())
}

func TestMinogrpc_GetAddress(t *testing.T) {
	addr := address{}
	minoGrpc := &Minogrpc{
		overlay: &overlay{me: addr},
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
	m.closing <- xerrors.New("oops")
	err = m.GracefulStop()
	require.EqualError(t, err, "server stopped unexpectedly: oops")

	m.closing = make(chan error)
	close(m.closing)
	m.connMgr = fakeConnMgr{len: 1}
	err = m.GracefulStop()
	require.EqualError(t, err, "connection manager not empty: 1")
}

func TestMinogrpc_MakeNamespace(t *testing.T) {
	minoGrpc := Minogrpc{}
	ns := "Test"
	newMino, err := minoGrpc.MakeNamespace(ns)
	require.NoError(t, err)

	newMinoGrpc, ok := newMino.(*Minogrpc)
	require.True(t, ok)

	require.Equal(t, ns, newMinoGrpc.namespace)

	// A namespace can not be empty
	ns = ""
	_, err = minoGrpc.MakeNamespace(ns)
	require.EqualError(t, err, "a namespace can not be empty")

	// A namespace should match [a-zA-Z0-9]+
	ns = "/namespace"
	_, err = minoGrpc.MakeNamespace(ns)
	require.EqualError(t, err, "a namespace should match [a-zA-Z0-9]+, but found '/namespace'")

	ns = " test"
	_, err = minoGrpc.MakeNamespace(ns)
	require.EqualError(t, err, "a namespace should match [a-zA-Z0-9]+, but found ' test'")

	ns = "test$"
	_, err = minoGrpc.MakeNamespace(ns)
	require.EqualError(t, err, "a namespace should match [a-zA-Z0-9]+, but found 'test$'")
}

func TestMinogrpc_MakeRPC(t *testing.T) {
	minoGrpc := Minogrpc{
		namespace: "namespace",
		overlay:   &overlay{},
		endpoints: make(map[string]*Endpoint),
	}

	handler := mino.UnsupportedHandler{}

	rpc, err := minoGrpc.MakeRPC("name", handler, fake.MessageFactory{})
	require.NoError(t, err)

	expectedRPC := &RPC{
		factory: fake.MessageFactory{},
		overlay: &overlay{},
		uri:     "namespace/name",
	}

	endpoint, ok := minoGrpc.endpoints[expectedRPC.uri]
	require.True(t, ok)
	require.Equal(t, handler, endpoint.Handler)
	require.Equal(t, expectedRPC, rpc)
}

func TestMinogrpc_String(t *testing.T) {
	minoGrpc := &Minogrpc{
		overlay: &overlay{me: fake.NewAddress(0)},
	}

	require.Equal(t, "fake.Address[0]", minoGrpc.String())
}
