package blockstore

import (
	"sync/atomic"

	"go.dedis.ch/dela/core/store/hashtree"
)

// TreeCache is a storage for a tree. It supports asynchronous calls.
//
// - implements blockstore.TreeCache
type treeCache struct {
	tree atomic.Value
}

// NewTreeCache creates a new cache with the given tree as the first value.
func NewTreeCache(tree hashtree.Tree) TreeCache {
	var value atomic.Value
	value.Store(tree)

	return &treeCache{
		tree: value,
	}
}

// Get implements blockstore.TreeCache. It returns the current value of the
// cache.
func (c *treeCache) Get() hashtree.Tree {
	return c.tree.Load().(hashtree.Tree)
}

// Set implements blockstore.TreeCache. It stores the new tree as the cache
// value but panic if it is nil.
func (c *treeCache) Set(tree hashtree.Tree) {
	c.tree.Store(tree)
}
