package minogrpc

import (
	fmt "fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/mino"
)

func Test_NewMinogrpc(t *testing.T) {
	id := "127.0.0.1:3333"

	minoRPC, err := NewMinogrpc(id)
	require.NoError(t, err)

	require.Equal(t, id, minoRPC.GetAddress().String())
	require.Equal(t, "", minoRPC.namespace)

	peer := Peer{
		Address:     id,
		Certificate: minoRPC.server.cert.Leaf,
	}

	require.Equal(t, peer, minoRPC.server.neighbours[id])
}

func Test_MakeNamespace(t *testing.T) {
	minoGrpc := Minogrpc{}
	ns := "Test"
	newMino, err := minoGrpc.MakeNamespace(ns)
	require.NoError(t, err)

	newMinoGrpc, ok := newMino.(Minogrpc)
	require.True(t, ok)

	require.Equal(t, ns, newMinoGrpc.namespace)

	// A namespace can not be empty
	ns = ""
	newMino, err = minoGrpc.MakeNamespace(ns)
	require.EqualError(t, err, "a namespace can not be empty")

	// A namespace should match [a-zA-Z0-9]+
	ns = "/namespace"
	newMino, err = minoGrpc.MakeNamespace(ns)
	require.EqualError(t, err, "a namespace should match [a-zA-Z0-9]+, but found '/namespace'")

	ns = " test"
	newMino, err = minoGrpc.MakeNamespace(ns)
	require.EqualError(t, err, "a namespace should match [a-zA-Z0-9]+, but found ' test'")

	ns = "test$"
	newMino, err = minoGrpc.MakeNamespace(ns)
	require.EqualError(t, err, "a namespace should match [a-zA-Z0-9]+, but found 'test$'")
}

func Test_Address(t *testing.T) {
	addr := address{
		id: "test",
	}
	minoGrpc := Minogrpc{
		server: Server{
			addr: addr,
		},
	}

	require.Equal(t, addr, minoGrpc.GetAddress())
}

func Test_MakeRPC(t *testing.T) {
	minoGrpc := Minogrpc{}
	minoGrpc.namespace = "namespace"
	minoGrpc.server = Server{
		handlers: make(map[string]mino.Handler),
	}

	handler := testSameHandler{}

	rpc, err := minoGrpc.MakeRPC("name", handler)
	require.NoError(t, err)

	expectedRPC := RPC{
		handler: handler,
		srv:     minoGrpc.server,
		uri:     fmt.Sprintf("namespace/name"),
	}

	h, ok := minoGrpc.server.handlers[expectedRPC.uri]
	require.True(t, ok)
	require.Equal(t, handler, h)

	require.Equal(t, expectedRPC, rpc)

}
