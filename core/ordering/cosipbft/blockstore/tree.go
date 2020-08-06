package blockstore

import "go.dedis.ch/dela/core/store/hashtree"

type treeCache struct {
	tree hashtree.Tree
}

func NewTreeCache(tree hashtree.Tree) TreeCache {
	return &treeCache{
		tree: tree,
	}
}

func (c *treeCache) Get() hashtree.Tree {
	return c.tree
}

func (c *treeCache) Set(tree hashtree.Tree) {
	c.tree = tree
}
