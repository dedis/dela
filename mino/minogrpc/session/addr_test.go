package session

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestAddress_GetDialAddress(t *testing.T) {
	addr := NewAddress("127.0.0.2")
	require.Equal(t, "127.0.0.2", addr.GetDialAddress())
}

func TestAddress_Equal(t *testing.T) {
	addr := NewAddress("127.0.0.1:2000")
	require.True(t, addr.Equal(addr))
	require.False(t, addr.Equal(Address{}))
	require.False(t, addr.Equal(fake.NewAddress(0)))

	orch := NewOrchestratorAddress(addr)
	require.True(t, orch.Equal(orch))
	require.False(t, addr.Equal(orch))

	wrapped := newWrapAddress(orch)
	require.True(t, wrapped.Equal(wrapped))
	require.True(t, wrapped.Equal(addr))
	require.True(t, addr.Equal(wrapped))
	require.True(t, wrapped.Equal(orch))
	require.False(t, wrapped.Equal(fake.NewAddress(0)))
}

func TestAddress_MarshalText(t *testing.T) {
	addr := NewAddress("127.0.0.1:2000")

	buffer, err := addr.MarshalText()
	require.NoError(t, err)
	require.Equal(t, "F127.0.0.1:2000", string(buffer))

	orch := NewOrchestratorAddress(addr)

	buffer, err = orch.MarshalText()
	require.NoError(t, err)
	require.Equal(t, "O127.0.0.1:2000", string(buffer))
}

func TestAddress_String(t *testing.T) {
	addr := NewAddress("127.0.0.1:2000")
	require.Equal(t, addr.host, addr.String())

	orch := NewOrchestratorAddress(addr)
	require.Equal(t, "Orchestrator:"+addr.host, orch.String())
}

func TestWrapAddress_Unwrap(t *testing.T) {
	addr := newWrapAddress(NewAddress("A"))

	require.Equal(t, NewAddress("A"), addr.Unwrap())
}

func TestAddressFactory_FromText(t *testing.T) {
	factory := AddressFactory{}

	addr := factory.FromText([]byte(orchestratorCode + "127.0.0.1:2000"))
	require.Equal(t, "127.0.0.1:2000", addr.(Address).host)
	require.True(t, addr.(Address).orchestrator)

	addr = factory.FromText(nil)
	require.Equal(t, "", addr.(Address).host)
	require.False(t, addr.(Address).orchestrator)

	addr = factory.FromText([]byte(followerCode + "127.0.0.1:2001"))
	require.Equal(t, "127.0.0.1:2001", addr.(Address).host)
	require.False(t, addr.(Address).orchestrator)
}
