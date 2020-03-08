package minoch

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/mino"
)

func TestMinoch_New(t *testing.T) {
	manager := NewManager()

	m1, err := NewMinoch(manager, "A")
	require.NoError(t, err)
	require.NotNil(t, m1)

	m2, err := NewMinoch(manager, "B")
	require.NoError(t, err)
	require.NotNil(t, m2)

	m3, err := NewMinoch(manager, "A")
	require.Error(t, err)
	require.Nil(t, m3)

	m4, err := NewMinoch(manager, "")
	require.Error(t, err)
	require.Nil(t, m4)
}

func TestMinoch_GetAddressFactory(t *testing.T) {
	m := &Minoch{}
	require.IsType(t, AddressFactory{}, m.GetAddressFactory())
}

func TestMinoch_GetAddress(t *testing.T) {
	manager := NewManager()

	m, err := NewMinoch(manager, "A")
	require.NoError(t, err)

	addr := m.GetAddress()
	require.Equal(t, "A", addr.String())
}

func TestMinoch_MakeNamespace(t *testing.T) {
	manager := NewManager()

	m, err := NewMinoch(manager, "A")
	require.NoError(t, err)

	m2, err := m.MakeNamespace("abc")
	require.NoError(t, err)
	require.Equal(t, m.identifier, m2.(*Minoch).identifier)
	require.Equal(t, "/abc", m2.(*Minoch).path)
}

func TestMinoch_MakeRPC(t *testing.T) {
	manager := NewManager()

	m, err := NewMinoch(manager, "A")
	require.NoError(t, err)

	rpc, err := m.MakeRPC("rpc1", testHandler{})
	require.NoError(t, err)
	require.NotNil(t, rpc)
}

type testHandler struct {
	mino.UnsupportedHandler
}
