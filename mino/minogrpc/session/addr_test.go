package session

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAddress_Equal(t *testing.T) {
	addr := Address{host: "127.0.0.1:2000"}

	require.True(t, addr.Equal(addr))
	require.False(t, addr.Equal(Address{}))
}

func TestAddress_MarshalText(t *testing.T) {
	addr := Address{host: "127.0.0.1:2000"}

	buffer, err := addr.MarshalText()
	require.NoError(t, err)
	require.Equal(t, append([]byte(followerCode), []byte(addr.host)...), buffer)
}

func TestAddress_String(t *testing.T) {
	addr := Address{host: "127.0.0.1:2000"}
	require.Equal(t, addr.host, addr.String())
}

func TestAddressFactory_FromText(t *testing.T) {
	factory := AddressFactory{}
	addr := factory.FromText([]byte(orchestratorCode + "127.0.0.1:2000"))

	require.Equal(t, "127.0.0.1:2000", addr.(Address).host)
}
