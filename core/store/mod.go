// Package store defines the primitives of a simple key/value storage.
//
// Documentation Last Review: 08.10.2020
//
package store

// Readable is the interface for a readable store.
type Readable interface {
	Get(key []byte) ([]byte, error)
}

// Writable is the interface for a writable store.
type Writable interface {
	Set(key []byte, value []byte) error

	Delete(key []byte) error
}

// Snapshot is a state of the store that can be read and write independently. A
// write is applied only to the snapshot reference.
type Snapshot interface {
	Readable
	Writable
}

// Transaction is a generic interface that store implementations can use to
// provide atomicity.
type Transaction interface {
	// OnCommit adds a callback to be executed after the transaction
	// successfully commits.
	OnCommit(func())
}
