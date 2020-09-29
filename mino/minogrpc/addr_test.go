package minogrpc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
	require.Equal(t, "\ue000", string(text))
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
