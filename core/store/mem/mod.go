package mem

import (
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/crypto"
	"golang.org/x/xerrors"
)

type item struct {
	value   []byte
	deleted bool
}

// Trie is an in-memory implementation of a store. It saves the updates in an
// internal store and only keep the updates of the current trie. When reading,
// it'll look up by following the parent trie if the key is not found.
//
// TODO: merkle trie + share
type Trie struct {
	parent      *Trie
	root        []byte
	dirty       bool
	store       map[string]item
	hashFactory crypto.HashFactory
}

// NewTrie creates a new in-memory trie.
func NewTrie() *Trie {
	return &Trie{
		parent:      nil,
		dirty:       true,
		store:       make(map[string]item),
		hashFactory: crypto.NewSha256Factory(),
	}
}

// Get implements store.Reader. It returns the value associated with the key if
// it exists by first checking the current store then recursively checking the
// parent up to the root. If nothing is found, it returns an error.
func (t *Trie) Get(key []byte) ([]byte, error) {
	str := string(key)

	item, found := t.store[str]
	if found {
		if item.deleted {
			return nil, xerrors.Errorf("item %#x not found", key)
		}

		return item.value, nil
	}

	if t.parent == nil {
		return nil, xerrors.Errorf("item %#x not found", key)
	}

	val, err := t.parent.Get(key)
	if err != nil {
		// The error is wrapped to avoid big recursive errors.
		return nil, err
	}

	return val, nil
}

// Set implements store.Writer. It writes the value to the store.
func (t *Trie) Set(key, value []byte) error {
	t.store[string(key)] = item{
		value: value,
	}

	t.dirty = true

	return nil
}

// Delete implements store.Writer. It removes a key from the store and flag it
// as deleted.
func (t *Trie) Delete(key []byte) error {
	// Even if the item does not exist, it must be flagged so that a parent
	// won't return the value.
	t.store[string(key)] = item{
		deleted: true,
	}

	t.dirty = true

	return nil
}

// GetRoot implements store.Trie.
func (t *Trie) GetRoot() []byte {
	return t.root
}

// GetShare implements store.Store.
func (t *Trie) GetShare(key []byte) (store.Share, error) {
	return nil, nil
}

// ComputeRoot implements store.ReadWriteTrie.
func (t *Trie) ComputeRoot() ([]byte, error) {
	h := t.hashFactory.New()

	_, err := h.Write(t.parent.root)
	if err != nil {
		return nil, xerrors.Errorf("couldn't write parent root: %v", err)
	}

	for key, value := range t.store {
		if value.deleted {
			continue
		}

		_, err = h.Write([]byte(key))
		if err != nil {
			return nil, xerrors.Errorf("couldn't write key: %v", err)
		}

		_, err = h.Write(value.value)
		if err != nil {
			return nil, xerrors.Errorf("couldn't write value: %v", err)
		}
	}

	t.root = h.Sum(nil)
	t.dirty = false

	return t.root, nil
}

// Stage implements store.Trie.
func (t *Trie) Stage(fn func(store.ReadWriteTrie) error) (store.Store, error) {
	trie := t.makeChild()

	err := fn(trie)
	if err != nil {
		return nil, err
	}

	if trie.dirty {
		_, err = trie.ComputeRoot()
		if err != nil {
			return nil, xerrors.Errorf("couldn't compute root: %v", err)
		}
	}

	return trie, nil
}

func (t *Trie) makeChild() *Trie {
	clone := NewTrie()
	clone.parent = t

	return clone
}
