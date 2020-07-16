// Package hashtree defines the specialization of the store as a Merkle tree. It
// allows the creation of proofs to demonstrate if a key/value pair is stored in
// the tree, or if it is not.
package hashtree

import "go.dedis.ch/dela/core/store"

// Path is a path along the tree to a key and its value, or none if the key is
// not set.
type Path interface {
	// GetKey returns the key of the share.
	GetKey() []byte

	// GetValue returns the value of the share, or nil if it is not set.
	GetValue() []byte

	// GetRoot returns the store root calculated from the key.
	GetRoot() []byte
}

// Tree is a specialization of a store. It uses the Merkle data structure to
// create a root hash that represents the state of the tree and can be used to
// create proof of inclusion/proof of absence.
type Tree interface {
	store.Readable

	// GetRoot returns the root hash of this tree.
	GetRoot() []byte

	// GetPath returns a path to a key and its value in the tree. It can be use
	// as a proof of inclusion or a proof of absence in the contraray.
	GetPath(key []byte) (Path, error)

	// Stage must create a writable tree from the current one that will be
	// pass to the callback then return it.
	Stage(func(store.Snapshot) error) (Tree, error)
}
