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

	ComputeRoot() ([]byte, error)
}

// Share is a part of the store that has a partial computation of the root and
// it can be completed to verify that a piece of data exists in the store.
type Share interface {
	GetValue() []byte

	ComputeRoot(key []byte) ([]byte, error)
}

// Store is a representation of the storage at a given point in time.
type Store interface {
	Reader

	// GetRoot returns the unique identifier for this trie.
	GetRoot() []byte

	// GetShare returns a share of the store that will allow one to verify the
	// integrity of a piece of data. It can be ensured by completed the share
	// with the missing data to compute the final root that should match.
	GetShare(key []byte) (Share, error)

	Stage(func(ReadWriteTrie) error) (Store, error)
}
