// Package mem implements the hash tree interface by following the merkle binary
// prefix tree described in:
//
// https://www.usenix.org/system/files/conference/usenixsecurity15/sec15-paper-melara.pdf
package mem

import (
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/hashtree"
	"go.dedis.ch/dela/crypto"
	"golang.org/x/xerrors"
)

// MerkleTree is an in-memory implementation of a trie. It saves the updates in
// an internal store and only keep the updates of the current trie. When
// reading, it'll look up by following the parent trie if the key is not found.
//
// - implements trie.Trie
type MerkleTree struct {
	tree        *Tree
	hashFactory crypto.HashFactory
}

// NewMerkleTree creates a new in-memory trie.
func NewMerkleTree() *MerkleTree {
	return &MerkleTree{
		tree:        NewTree(32),
		hashFactory: crypto.NewSha256Factory(),
	}
}

// Get implements store.Readable.
func (t *MerkleTree) Get(key []byte) ([]byte, error) {
	value, err := t.tree.Search(key, nil)
	if err != nil {
		return nil, xerrors.Errorf("couldn't search key: %v", err)
	}

	return value, nil
}

// Set implements store.Writable.
func (t *MerkleTree) Set(key, value []byte) error {
	err := t.tree.Insert(key, value)
	if err != nil {
		return xerrors.Errorf("couldn't insert pair: %v", err)
	}

	return nil
}

// Delete implements store.Writable.
func (t *MerkleTree) Delete(key []byte) error {
	err := t.tree.Delete(key)
	if err != nil {
		return xerrors.Errorf("couldn't delete key: %v", err)
	}

	return nil
}

// GetRoot implements trie.Trie.
func (t *MerkleTree) GetRoot() []byte {
	return t.tree.root.GetHash()
}

// GetPath implements trie.Trie.
func (t *MerkleTree) GetPath(key []byte) (hashtree.Path, error) {
	path := newPath(key)

	_, err := t.tree.Search(key, &path)
	if err != nil {
		return nil, err
	}

	return path, nil
}

// Stage implements trie.Trie. It executes the callback over a child of the
// current trie and return the trie with the root calculated.
func (t *MerkleTree) Stage(fn func(store.Snapshot) error) (hashtree.Tree, error) {
	trie := t.clone()

	err := fn(trie)
	if err != nil {
		return nil, xerrors.Errorf("callback failed: %v", err)
	}

	err = t.tree.Update()
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
