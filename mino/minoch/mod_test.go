package minoch

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
)

func TestMinoch_New(t *testing.T) {
	manager := NewManager()

	m1 := NewMinoch(manager, "A")
	require.NotNil(t, m1)

	m2 := NewMinoch(manager, "B")
	require.NotNil(t, m2)
}

func TestMinoch_SameIdentifier_New(t *testing.T) {
	defer func() {
		r := recover()
		require.Equal(t, r, "manager refused: identifier <A> already exists")
	}()

	manager := NewManager()

	NewMinoch(manager, "A")
	NewMinoch(manager, "A")
}

func TestMinoch_GetAddressFactory(t *testing.T) {
	m := &Minoch{}
	require.IsType(t, AddressFactory{}, m.GetAddressFactory())
}

func TestMinoch_GetAddress(t *testing.T) {
	manager := NewManager()

	m := NewMinoch(manager, "A")

	addr := m.GetAddress()
	require.Equal(t, "A", addr.String())
}

func TestMinoch_AddFilter(t *testing.T) {
	manager := NewManager()

	m := NewMinoch(manager, "A")

	rpc, err := m.MakeRPC("test", nil, nil)
	require.NoError(t, err)

	m.AddFilter(func(m mino.Request) bool { return true })
	require.Len(t, m.filters, 1)
	require.Len(t, rpc.(*RPC).filters, 1)
}

func TestMinoch_MakeNamespace(t *testing.T) {
	manager := NewManager()

	m := NewMinoch(manager, "A")

	m2, err := m.MakeNamespace("abc")
	require.NoError(t, err)
	require.Equal(t, m.identifier, m2.(*Minoch).identifier)
	require.Equal(t, "/abc", m2.(*Minoch).path)
}

func TestMinoch_MakeRPC(t *testing.T) {
	manager := NewManager()

	m := NewMinoch(manager, "A")

	rpc, err := m.MakeRPC("rpc1", badHandler{}, fake.MessageFactory{})
	require.NoError(t, err)
	require.NotNil(t, rpc)
}

type badHandler struct {
	mino.UnsupportedHandler
}

type fakeHandler struct {
	mino.UnsupportedHandler
}

func (h fakeHandler) Process(req mino.Request) (resp serde.Message, err error) {
	return fake.Message{}, nil
}
