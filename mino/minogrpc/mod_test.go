package minogrpc

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	internal "go.dedis.ch/fabric/internal/testing"
	"go.dedis.ch/fabric/mino"
)

func TestMessages(t *testing.T) {
	messages := []proto.Message{
		&Envelope{},
	}

	for _, m := range messages {
		internal.CoverProtoMessage(t, m)
	}
}

func TestMinogrpc_New(t *testing.T) {
	m, err := NewMinogrpc("127.0.0.1", 3333, nil)
	require.NoError(t, err)

	require.Equal(t, "127.0.0.1:3333", m.GetAddress().String())
	require.Equal(t, "", m.namespace)

	cert := m.GetCertificate()
	require.NotNil(t, cert)

	// Giving a wrong address, should be "//example:3333"
	_, err = NewMinogrpc("\\", 0, nil)
	require.EqualError(t, err,
		"couldn't parse url: parse \"//\\\\:0\": invalid character \"\\\\\" in host name")
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

func TestMinogrpc_GetAddress(t *testing.T) {
	addr := address{}
	minoGrpc := Minogrpc{
		overlay: overlay{me: addr},
	}

	require.Equal(t, addr, minoGrpc.GetAddress())
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

func TestMinogrpc_GetAddressFactory(t *testing.T) {
	m := &Minogrpc{}
	require.IsType(t, AddressFactory{}, m.GetAddressFactory())
}
