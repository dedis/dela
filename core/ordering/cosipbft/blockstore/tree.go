// This file contains the implementation of an in-memory tree cache.
//
// Documenation Last Review: 13.10.2020
//

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

// GetWithLock implements blockstore.TreeCache. It returns the current value of
// the cache alongside a function to unlock the cache. It allows one to delay
// a set while fetching associated data. The function returned must be called.
func (c *treeCache) GetWithLock() (hashtree.Tree, func()) {
	c.Lock()

	return c.tree, c.unlock
}

// Set implements blockstore.TreeCache. It stores the new tree as the cache
// value but panic if it is nil.
func (c *treeCache) Set(tree hashtree.Tree) {
	c.Lock()
	c.tree = tree
	c.Unlock()
}

// SetWithLock implements blockstore.TreeCache. It sets the tree while holding
// the lock and returns a function to unlock it. It allows one to prevent an
// access until associated data is updated. The function returned must be
// called.
func (c *treeCache) SetWithLock(tree hashtree.Tree) func() {
	c.Lock()
	c.tree = tree

	return c.unlock
}

func (c *treeCache) unlock() {
	c.Unlock()
}
