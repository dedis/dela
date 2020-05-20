package minogrpc

import (
	"net/url"
	"sync"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	internal "go.dedis.ch/fabric/internal/testing"
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/fabric/mino/minogrpc/routing"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
)

func TestMessages(t *testing.T) {
	messages := []proto.Message{
		&Envelope{},
		&Message{},
	}

	for _, m := range messages {
		internal.CoverProtoMessage(t, m)
	}
}

func TestRootAddress_Equal(t *testing.T) {
	root := newRootAddress()
	require.True(t, root.Equal(newRootAddress()))
	require.True(t, root.Equal(root))
	require.False(t, root.Equal(address{}))
}

func TestRootAddress_MarshalText(t *testing.T) {
	root := newRootAddress()
	text, err := root.MarshalText()
	require.NoError(t, err)
	require.Equal(t, []byte{}, text)
}

func TestRootAddress_String(t *testing.T) {
	root := newRootAddress()
	require.Equal(t, orchestratorDescription, root.String())
}

func TestAddress_Equal(t *testing.T) {
	addr := address{host: "127.0.0.1:2000"}

	require.True(t, addr.Equal(addr))
	require.False(t, addr.Equal(address{}))
}

func TestAddress_MarshalText(t *testing.T) {
	addr := address{host: "127.0.0.1:2000"}
	buffer, err := addr.MarshalText()
	require.NoError(t, err)

	require.Equal(t, []byte(addr.host), buffer)
}

func TestAddress_String(t *testing.T) {
	addr := address{host: "127.0.0.1:2000"}
	require.Equal(t, addr.host, addr.String())
}

func TestAddressFactory_FromText(t *testing.T) {
	factory := AddressFactory{}
	addr := factory.FromText([]byte("127.0.0.1:2000"))

	require.Equal(t, "127.0.0.1:2000", addr.(address).host)
}

func TestMinogrpc_New(t *testing.T) {
	m, err := NewMinogrpc("127.0.0.1", 3333, routing.NewTreeRoutingFactory(1, AddressFactory{}))
	require.NoError(t, err)

	require.Equal(t, "127.0.0.1:3333", m.GetAddress().String())
	require.Equal(t, "", m.namespace)

	cert := m.GetCertificate()
	require.NotNil(t, cert)

	require.NoError(t, m.GracefulClose())

	// Giving a wrong address, should be "//example:3333"
	_, err = NewMinogrpc("\\", 0, nil)
	require.EqualError(t, err,
		"couldn't parse url: parse \"//\\\\:0\": invalid character \"\\\\\" in host name")

	_, err = NewMinogrpc("127.0.0.1", 1, routing.NewTreeRoutingFactory(1, AddressFactory{}))
	require.EqualError(t, err,
		"couldn't start the server: failed to listen: listen tcp4 127.0.0.1:1: bind: permission denied")
}

func TestMinogrpc_GetAddressFactory(t *testing.T) {
	m := &Minogrpc{}
	require.IsType(t, AddressFactory{}, m.GetAddressFactory())
}

func TestMinogrpc_GetAddress(t *testing.T) {
	addr := address{}
	minoGrpc := Minogrpc{
		overlay: overlay{me: addr},
	}

	require.Equal(t, addr, minoGrpc.GetAddress())
}

func TestMinogrpc_GracefulClose(t *testing.T) {
	m, err := NewMinogrpc("127.0.0.1", 0, routing.NewTreeRoutingFactory(1, AddressFactory{}))
	require.NoError(t, err)

	require.NoError(t, m.GracefulClose())

	// Closing when the server has not started.
	m = &Minogrpc{started: make(chan struct{})}
	err = m.GracefulClose()
	require.EqualError(t, err, "server should be listening before trying to close")

	// gRPC failed to stop gracefully.
	m = &Minogrpc{
		url:     &url.URL{Host: "127.0.0.1:0"},
		server:  grpc.NewServer(),
		started: make(chan struct{}),
		closer:  &sync.WaitGroup{},
		closing: make(chan error, 1),
	}
	m.closer.Add(1)
	close(m.started)
	m.closing <- xerrors.New("oops")

	err = m.GracefulClose()
	require.EqualError(t, err, "failed to stop gracefully: oops")
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
		overlay:   overlay{},
		handlers:  make(map[string]mino.Handler),
	}

	handler := mino.UnsupportedHandler{}

	rpc, err := minoGrpc.MakeRPC("name", handler)
	require.NoError(t, err)

	expectedRPC := &RPC{
		overlay: overlay{},
		uri:     "namespace/name",
	}

	h, ok := minoGrpc.handlers[expectedRPC.uri]
	require.True(t, ok)
	require.Equal(t, handler, h)
	require.Equal(t, expectedRPC, rpc)
}
