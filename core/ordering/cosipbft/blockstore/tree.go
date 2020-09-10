package blockstore

import (
	"sync"

	"go.dedis.ch/dela/core/store/hashtree"
)

// TreeCache is a storage for a tree. It supports asynchronous calls.
//
// - implements blockstore.TreeCache
type treeCache struct {
	sync.Mutex
	tree hashtree.Tree
}

// NewTreeCache creates a new cache with the given tree as the first value.
func NewTreeCache(tree hashtree.Tree) TreeCache {
	return &treeCache{
		tree: tree,
	}
}

// Get implements blockstore.TreeCache. It returns the current value of the
// cache.
func (c *treeCache) Get() hashtree.Tree {
	c.Lock()
	defer c.Unlock()

	return c.tree
}

// Set implements blockstore.TreeCache. It stores the new tree as the cache
// value but panic if it is nil.
func (c *treeCache) Set(tree hashtree.Tree) {
	c.Lock()
	c.tree = tree
	c.Unlock()
}

// SetAndLock implements blockstore.TreeCache. It sets the tree and acquires the
// cache lock until the wait group is done which allows an external caller to
// delay the release of the lock.
func (c *treeCache) SetAndLock(tree hashtree.Tree, lock *sync.WaitGroup) {
	c.Lock()
	c.tree = tree

	go func() {
		// This allows an external call to hold the cache until some event are
		// fulfilled.
		lock.Wait()

		c.Unlock()
	}()
}
