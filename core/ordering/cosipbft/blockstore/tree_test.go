package blockstore

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/store/hashtree"
)

func TestTreeCache_Get(t *testing.T) {
	cache := NewTreeCache(fakeTree{})

	require.Equal(t, fakeTree{}, cache.Get())
}

func TestTreeCache_Set(t *testing.T) {
	cache := NewTreeCache(fakeTree{})

	cache.Set(fakeTree{value: 1})
	require.Equal(t, fakeTree{value: 1}, cache.Get())
}

// Utility functions
// -----------------------------------------------------------------------------

type fakeTree struct {
	hashtree.Tree
	value int
}
