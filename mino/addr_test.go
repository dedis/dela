package mino

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAddressIterator_Seek(t *testing.T) {
	addrs := []Address{fakeAddr{}, fakeAddr{}}

	iter := addressIterator{addrs: addrs}
	iter.Seek(1)
	require.NotNil(t, iter.GetNext())
	require.False(t, iter.HasNext())
}

func TestAddressIterator_HasNext(t *testing.T) {
	addrs := []Address{fakeAddr{}, fakeAddr{}}

	iter := addressIterator{addrs: addrs}
	require.True(t, iter.HasNext())

	iter.GetNext()
	iter.GetNext()
	require.False(t, iter.HasNext())
}

func TestAddressIterator_GetNext(t *testing.T) {
	addrs := []Address{fakeAddr{}, fakeAddr{}}

	iter := addressIterator{addrs: addrs}
	require.NotNil(t, iter.GetNext())
	require.NotNil(t, iter.GetNext())
	require.False(t, iter.HasNext())
}

func TestRoster_Take(t *testing.T) {
	addrs := NewAddresses(fakeAddr{}, fakeAddr{}, fakeAddr{})

	addrs2 := addrs.Take(IndexFilter(0), IndexFilter(2))
	require.Equal(t, 2, len(addrs2.(roster).addrs))
}

func TestRoster_AddressIterator(t *testing.T) {
	addrs := NewAddresses(fakeAddr{})

	iter := addrs.AddressIterator()
	require.NotNil(t, iter.GetNext())
	require.False(t, iter.HasNext())
}

func TestRoster_Len(t *testing.T) {
	addrs := NewAddresses(fakeAddr{}, fakeAddr{})
	require.Equal(t, 2, addrs.Len())
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeAddr struct {
	Address
}
