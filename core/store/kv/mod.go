// Package kv defines the abstraction for a key/value database.
//
// The package also implements a default database implementation that is using
// bbolt as the engine (https://github.com/etcd-io/bbolt).
//
// Documentation Last Review: 08.10.2020
//
package kv

import "go.dedis.ch/dela/core/store"

// Bucket is a general interface to operate on a database bucket.
type Bucket interface {
	// Get reads the key from the bucket and returns the value, or nil if the
	// key does not exist.
	Get(key []byte) []byte

	// Set assigns the value to the provided key.
	Set(key, value []byte) error

	// Delete deletes the key from the bucket.
	Delete(key []byte) error

	// ForEach iterates over all the items in the bucket in a unspecified order.
	// The iteration stops when the callback returns an error.
	ForEach(func(k, v []byte) error) error

	// Scan iterates over every key that matches the prefix in an order
	// determined by the implementation. The iteration stops when the callback
	// returns an error.
	Scan(prefix []byte, fn func(k, v []byte) error) error
}

// ReadableTx allows one to perform read-only atomic operations on the database.
type ReadableTx interface {
	// GetBucket returns the bucket of the given name if it exists, otherwise it
	// returns nil.
	GetBucket(name []byte) Bucket
}

// WritableTx allows one to perform atomic operations on the database.
type WritableTx interface {
	store.Transaction

	ReadableTx

	// GetBucketOrCreate returns the bucket of the given name if it exists, or
	// it creates it.
	GetBucketOrCreate(name []byte) (Bucket, error)
}

// DB is a general interface to operate over a key/value database.
type DB interface {
	// View executes the provided read-only transaction in the context of the
	// database.
	View(fn func(ReadableTx) error) error

	// Update executes the provided writable transaction in the context of the
	// database.
	Update(fn func(WritableTx) error) error

	// Close closes the database and free the resources.
	Close() error
}
