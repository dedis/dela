package minogrpc

import (
	fmt "fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/mino"
)

func Test_MakeMinoGrpc(t *testing.T) {
	id := "127.0.0.1:3333"

	minoRPC, err := MakeMinoGrpc(id)
	require.NoError(t, err)

	require.Equal(t, id, minoRPC.Address().GetId())
	require.Equal(t, "", minoRPC.namespace)

	peer := Peer{
		Address:     id,
		Certificate: minoRPC.server.cert.Leaf,
	}

	require.Equal(t, peer, minoRPC.server.neighbours[id])
}

func Test_MakeNamespace(t *testing.T) {
	minoGrpc := Minogrpc{}
	ns := "test"
	newMino, err := minoGrpc.MakeNamespace(ns)
	require.NoError(t, err)

	newMinoGrpc, ok := newMino.(Minogrpc)
	require.True(t, ok)

	require.Equal(t, ns, newMinoGrpc.namespace)
}

func Test_Address(t *testing.T) {
	addr := &mino.Address{
		Id: "test",
	}
	minoGrpc := Minogrpc{
		server: Server{
			addr: addr,
		},
	}

	require.Equal(t, addr, minoGrpc.Address())
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
