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
}

// Share is a part of the store that has a partial computation of the root so
// that it can be calculated from the key/value pair to ensure the integrity.
type Share interface {
	// GetKey returns the key of the share.
	GetKey() []byte

	// GetValue returns the value of the share, or nil if it is not set.
	GetValue() []byte

	// GetRoot returns the store root calculated from the key.
	GetRoot() []byte
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

	// Stage must create a writable trie from the current one that will be
	// pasto the callback then return it.
	Stage(func(ReadWriteTrie) error) (Store, error)
}
