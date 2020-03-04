package minogrpc

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/mino"
)

func Test_MakeMinoGrpc(t *testing.T) {
	id := "127.0.0.1:3333"

	server, err := MakeMinoGrpc(id)
	require.NoError(t, err)

	require.Equal(t, id, server.Address().GetId())
	require.Equal(t, "", server.namespace)
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
