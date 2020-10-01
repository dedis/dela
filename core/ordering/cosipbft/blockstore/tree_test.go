package blockstore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/store/hashtree"
)

func TestTreeCache_Get(t *testing.T) {
	cache := NewTreeCache(fakeTree{})

	require.Equal(t, fakeTree{}, cache.Get())
}

func TestTreeCache_GetWithLock(t *testing.T) {
	cache := NewTreeCache(fakeTree{})

	tree, unlock := cache.GetWithLock()
	require.NotNil(t, tree)

	unlock()

	tree, unlock = cache.GetWithLock()
	require.NotNil(t, tree)

	unlock()
}

func TestTreeCache_Set(t *testing.T) {
	cache := NewTreeCache(fakeTree{})

	cache.Set(fakeTree{value: 1})
	require.Equal(t, fakeTree{value: 1}, cache.Get())
}

func TestTreeCache_SetAndLock(t *testing.T) {
	cache := NewTreeCache(fakeTree{})

	unlock := cache.SetWithLock(fakeTree{})

	ch := make(chan struct{})
	go func() {
		cache.Get()
		close(ch)
	}()

	time.Sleep(50 * time.Millisecond)

	select {
	case <-ch:
		t.Fatal("get should be locked")
	default:
	}

	unlock()

	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("get should be released")
	}
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeTree struct {
	hashtree.Tree
	value int
}
