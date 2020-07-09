package mem

import "go.dedis.ch/dela/core/store"

// Trie is an in-memory implementation of a trie. It saves the updates in an
// internal store and only keep the updates of the current trie. When reading,
// it'll look up by following the parent trie if the key is not found.
type Trie struct {
	parent *Trie
	store  map[string][]byte
}

// Get implements store.Reader.
func (t *Trie) Get(key []byte) ([]byte, error) {
	str := string(key)

	val := t.store[str]
	if val != nil {
		return val, nil
	}

	val, err := t.parent.Get(key)
	if err != nil {
		return nil, err
	}

	return val, nil
}

// Set implements store.Writer.
func (t *Trie) Set(key, value []byte) error {
	t.store[string(key)] = value

	return nil
}

// Delete implements store.Writer.
func (t *Trie) Delete(key []byte) error {
	delete(t.store, string(key))

	return nil
}

// GetRoot implements store.Trie.
func (t *Trie) GetRoot() []byte {
	return []byte{}
}

// ComputeRoot implements store.ReadWriteTrie.
func (t *Trie) ComputeRoot() []byte {
	return []byte{}
}

// Stage implements store.Trie.
func (t *Trie) Stage(fn func(store.ReadWriteTrie) error) (store.Trie, error) {
	trie := t.makeChild()

	err := fn(trie)
	if err != nil {
		return nil, err
	}

	return trie, nil
}

func (t *Trie) makeChild() *Trie {
	clone := &Trie{
		parent: t,
		store:  make(map[string][]byte),
	}

	return clone
}
