package store

// Reader is a reader interface for a trie.
type Reader interface {
	Get(key []byte) ([]byte, error)
}

// Writer is a writer interface for a trie.
type Writer interface {
	Set(key []byte, value []byte) error

	Delete(key []byte) error
}

// ReadWriteTrie is a staging trie interface to update an existing one and
// create a new child trie.
type ReadWriteTrie interface {
	Reader
	Writer

	ComputeRoot() []byte
}

// Trie is a representation of the storage at a given point in time.
type Trie interface {
	Reader

	// GetRoot returns the unique identifier for this trie.
	GetRoot() []byte

	Stage(func(ReadWriteTrie) error) (Trie, error)
}
