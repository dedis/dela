package minoch

import (
	"bytes"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/mino"
)

func TestAddress_Equal(t *testing.T) {
	addr := address{id: "A"}
	require.True(t, addr.Equal(addr))
	require.False(t, addr.Equal(address{}))
	require.False(t, addr.Equal(fakeAddress{}))
}

func TestAddress_MarshalText(t *testing.T) {
	f := func(id string) bool {
		addr := address{id: id}
		buffer, err := addr.MarshalText()
		require.NoError(t, err)

		return bytes.Equal([]byte(id), buffer)
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestAddress_String(t *testing.T) {
	f := func(id string) bool {
		addr := address{id: id}

		return id == addr.String()
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestAddressFactory_FromText(t *testing.T) {
	f := func(id string) bool {
		factory := AddressFactory{}
		addr := factory.FromText([]byte(id))

		return addr.(address).id == id
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

//------------------------------------------------------------------------------
// Utility functions

type fakeAddress struct {
	mino.Address
}
