package mem

import (
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/trie"
	"go.dedis.ch/dela/crypto"
	"golang.org/x/xerrors"
)

// Trie is an in-memory implementation of a trie. It saves the updates in an
// internal store and only keeps the updates of the current trie. When reading,
// it'll look up by following the parent trie if the key is not found.
//
// - implements trie.Trie
type Trie struct {
	tree        *Tree
	hashFactory crypto.HashFactory
}

// NewTrie creates a new in-memory trie.
func NewTrie() *Trie {
	return &Trie{
		tree:        NewTree(32),
		hashFactory: crypto.NewSha256Factory(),
	}
}

// Get implements store.Readable.
func (t *Trie) Get(key []byte) ([]byte, error) {
	value, err := t.tree.Search(key, nil)
	if err != nil {
		return nil, xerrors.Errorf("couldn't search key: %v", err)
	}

	return value, nil
}

// Set implements store.Writable.
func (t *Trie) Set(key, value []byte) error {
	err := t.tree.Insert(key, value)
	if err != nil {
		return xerrors.Errorf("couldn't insert pair: %v", err)
	}

	return nil
}

// Delete implements store.Writable.
func (t *Trie) Delete(key []byte) error {
	err := t.tree.Delete(key)
	if err != nil {
		return xerrors.Errorf("couldn't delete key: %v", err)
	}

	return nil
}

// GetRoot implements trie.Trie.
func (t *Trie) GetRoot() []byte {
	return t.tree.root.GetHash()
}

// GetPath implements trie.Trie.
func (t *Trie) GetPath(key []byte) (trie.Path, error) {
	path := newPath(key)

	_, err := t.tree.Search(key, &path)
	if err != nil {
		return nil, err
	}

	return path, nil
}

// Stage implements trie.Trie. It executes the callback over a child of the
// current trie and return the trie with the root calculated.
func (t *Trie) Stage(fn func(store.Snapshot) error) (trie.Trie, error) {
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

func (t *Trie) clone() *Trie {
	return &Trie{
		tree:        t.tree.Clone(),
		hashFactory: t.hashFactory,
	}
}
