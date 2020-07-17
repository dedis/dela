// Package trie defines the specialization of the store as a trie. It allows the
// creation of proofs to demonstrate if a key/value pair is stored in the trie,
// or if it is not.
package trie

import "go.dedis.ch/dela/core/store"

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

// Trie is a specialization of a store. It uses the trie data structure to
// create a root hash that represents the state of the trie and can be used to
// create proof of existance/proof of inexistance.
type Trie interface {
	store.Readable

	// GetRoot returns the root hash of this trie.
	GetRoot() []byte

	// GetShare returns a share of the store that will allow one to verify the
	// integrity of a piece of data. It can be ensured by completing the share
	// with the missing data to compute the final root that should match.
	GetShare(key []byte) (Share, error)

	// Stage must create a writable trie from the current one that will be
	// passed to the callback then return it.
	Stage(func(store.Snapshot) error) (Trie, error)
}
