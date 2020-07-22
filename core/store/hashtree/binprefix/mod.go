// Package binprefix implements the hash tree interface by following the merkle
// binary prefix tree described in:
//
// @nolint-next-line
// https://www.usenix.org/system/files/conference/usenixsecurity15/sec15-paper-melara.pdf
//
// The merkle tree is stored in-memory until it reaches a certain threshold of
// depth where it will write the nodes in disk. The leaf are always stored in
// disk because of the value it holds.
//
//                         Interior (Root)
//                           /        \
//                        0 /          \ 1
//                         /            \
//                      Interior         Empty
//                       /    \          /   \
//                    0 /      \ 1    0 /     \ 1
//                     /        \      /       \
//                DiskNode  Interior  Empty    Interior
//                           /   \              /    \
// -------------------------------------------------------------- Memory Depth
//                       0 /       \ 1      0 /        \ 1
//                        /         \        /          \
//                    DiskNode  DiskNode  DiskNode   DiskNode
//
// The drawing above demonstrates an example of a tree. Here the memory depth is
// set at 3 which means that every node after this level will be a disk node. It
// will be loaded to its in-memory type when traversing the tree. The node at
// prefix 00 is an example of a leaf node which is a disk node even above the
// memory depth level.
package binprefix

import (
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/hashtree"
	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/crypto"
	"golang.org/x/xerrors"
)

// MerkleTree is an in-memory implementation of a Merkle prefix binary tree.
// This particular implementation assumes the keys will have the same length so
// that only the longest unique prefix along the path can be a leaf node.
//
// - implements hashtree.Tree
type MerkleTree struct {
	tree        *Tree
	db          kv.DB
	bucket      []byte
	hashFactory crypto.HashFactory
}

// NewMerkleTree creates a new in-memory trie.
func NewMerkleTree(db kv.DB, nonce Nonce) *MerkleTree {
	return &MerkleTree{
		tree:        NewTree(nonce),
		db:          db,
		bucket:      []byte("hashtree"),
		hashFactory: crypto.NewSha256Factory(),
	}
}

// Get implements store.Readable. It returns the value associated with the key
// if it exists, otherwise it returns nil.
func (t *MerkleTree) Get(key []byte) ([]byte, error) {
	var value []byte

	err := t.db.View(t.bucket, func(b kv.Bucket) error {
		var err error
		value, err = t.tree.Search(key, nil, b)

		return err
	})

	if err != nil {
		return nil, xerrors.Errorf("couldn't search key: %v", err)
	}

	return value, nil
}

// GetRoot implements hashtree.Tree. It returns the root of the hash tree.
func (t *MerkleTree) GetRoot() []byte {
	return t.tree.root.GetHash()
}

// GetPath implements hashtree.Tree. It returns a path to a given key that can
// be used to prove the inclusion or the absence of a key.
func (t *MerkleTree) GetPath(key []byte) (hashtree.Path, error) {
	path := newPath(t.tree.nonce[:], key)

	err := t.db.View(t.bucket, func(b kv.Bucket) error {
		_, err := t.tree.Search(key, &path, b)
		return err
	})

	if err != nil {
		return nil, xerrors.Errorf("couldn't search key: %v", err)
	}

	return path, nil
}

// Stage implements hashtree.Tree. It executes the callback over a clone of the
// current tree and return the clone with the root calculated.
func (t *MerkleTree) Stage(fn func(store.Snapshot) error) (hashtree.StagingTree, error) {
	clone := t.clone()

	err := t.db.Update(t.bucket, func(b kv.Bucket) error {
		err := fn(writableMerkleTree{
			MerkleTree: clone,
			bucket:     b,
		})

		if err != nil {
			return xerrors.Errorf("callback failed: %v", err)
		}

		err = clone.tree.Update(t.hashFactory, b)
		if err != nil {
			return xerrors.Errorf("couldn't update tree: %v", err)
		}

		return nil
	})

	return clone, err
}

// Commit implements hashtree.StagingTree. It writes the leaf nodes to the disk
// and a trade-off of other nodes.
func (t *MerkleTree) Commit() (hashtree.Tree, error) {
	err := t.db.Update(t.bucket, func(b kv.Bucket) error {
		return t.tree.Persist(b)
	})

	if err != nil {
		return nil, xerrors.Errorf("failed to persist tree: %v", err)
	}

	return t, nil
}

func (t *MerkleTree) clone() *MerkleTree {
	return &MerkleTree{
		tree:        t.tree.Clone(),
		db:          t.db,
		bucket:      t.bucket,
		hashFactory: t.hashFactory,
	}
}

// WritableMerkleTree is a wrapper around the merkle tree implementation so that
// it can be written into but the tree is not updated at every operation.
//
// - implements store.Writable
type writableMerkleTree struct {
	*MerkleTree

	bucket kv.Bucket
}

// Set implements store.Writable. It adds or updates the key in the internal
// tree.
func (t writableMerkleTree) Set(key, value []byte) error {
	err := t.tree.Insert(key, value, t.bucket)
	if err != nil {
		return xerrors.Errorf("couldn't insert pair: %v", err)
	}

	return nil
}

// Delete implements store.Writable. It removes the key from the tree.
func (t writableMerkleTree) Delete(key []byte) error {
	err := t.tree.Delete(key, t.bucket)
	if err != nil {
		return xerrors.Errorf("couldn't delete key: %v", err)
	}

	return nil
}
