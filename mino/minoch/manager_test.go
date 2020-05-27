package minoch

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
)

func TestManager_Get(t *testing.T) {
	manager := &Manager{
		instances: map[string]*Minoch{"A": {}},
	}

	m, err := manager.get(address{id: "A"})
	require.NoError(t, err)
	require.NotNil(t, m)

	_, err = manager.get(address{id: "B"})
	require.EqualError(t, err, "address <B> not found")

	_, err = manager.get(fake.NewBadAddress())
	require.EqualError(t, err, "invalid address type 'fake.Address'")
}

func TestManager_Insert(t *testing.T) {
	manager := NewManager()

	err := manager.insert(&Minoch{identifier: "A"})
	require.NoError(t, err)

	err = manager.insert(&Minoch{identifier: "A"})
	require.EqualError(t, err, "identifier <A> already exists")

	err = manager.insert(&Minoch{})
	require.EqualError(t, err, "cannot have an empty identifier")

	err = manager.insert(fakeInstance{})
	require.EqualError(t, err, "invalid instance type 'minoch.fakeInstance'")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeInstance struct {
	mino.Mino
}
