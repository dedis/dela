// Package mem implements the hash tree interface by following the merkle binary
// prefix tree described in:
//
// https://www.usenix.org/system/files/conference/usenixsecurity15/sec15-paper-melara.pdf
package mem

import (
	"crypto/rand"

	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/hashtree"
	"go.dedis.ch/dela/crypto"
	"golang.org/x/xerrors"
)

// MerkleTree is an in-memory implementation of a Merkle prefix binary tree.
// This particular implementation assumes the keys will have the same length so
// that any prefix is overriden by the index.
//
// - implements hashtree.Tree
type MerkleTree struct {
	tree        *Tree
	hashFactory crypto.HashFactory
}

// NewMerkleTree creates a new in-memory trie.
func NewMerkleTree() *MerkleTree {
	nonce := Nonce{}
	rand.Read(nonce[:])

	return &MerkleTree{
		tree:        NewTree(nonce),
		hashFactory: crypto.NewSha256Factory(),
	}
}

// Get implements store.Readable. It returns the value associated with the key
// it exists, otherwise it returns nil.
func (t *MerkleTree) Get(key []byte) ([]byte, error) {
	value, err := t.tree.Search(key, nil)
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
	path := newPath(key)

	_, err := t.tree.Search(key, &path)
	if err != nil {
		return nil, xerrors.Errorf("couldn't search key: %v", err)
	}

	return path, nil
}

// Stage implements hashtree.Tree. It executes the callback over a child of the
// current trie and return the trie with the root calculated.
func (t *MerkleTree) Stage(fn func(store.Snapshot) error) (hashtree.Tree, error) {
	trie := t.clone()

	err := fn(writableMerkleTree{MerkleTree: trie})
	if err != nil {
		return nil, xerrors.Errorf("callback failed: %v", err)
	}

	err = trie.tree.Update(t.hashFactory)
	if err != nil {
		return nil, xerrors.Errorf("couldn't update tree: %v", err)
	}

	return trie, nil
}

func (t *MerkleTree) clone() *MerkleTree {
	return &MerkleTree{
		tree:        t.tree.Clone(),
		hashFactory: t.hashFactory,
	}
}

// WritableMerkleTree is a wrapper around the merkle tree implementation so that
// it can be written into but the tree is not updated at every operation.
//
// - implements store.Writable
type writableMerkleTree struct {
	*MerkleTree
}

// Set implements store.Writable. It adds or updates the key in the internal
// tree.
func (t writableMerkleTree) Set(key, value []byte) error {
	err := t.tree.Insert(key, value)
	if err != nil {
		return xerrors.Errorf("couldn't insert pair: %v", err)
	}

	return nil
}

// Delete implements store.Writable. It removes the key from the tree.
func (t writableMerkleTree) Delete(key []byte) error {
	err := t.tree.Delete(key)
	if err != nil {
		return xerrors.Errorf("couldn't delete key: %v", err)
	}

	return nil
}
