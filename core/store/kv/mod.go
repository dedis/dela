package kv

import "go.dedis.ch/dela/core/store"

// Bucket is a general interface to operate on a database bucket.
type Bucket interface {
	Get(key []byte) []byte

	Set(key, value []byte) error

	Delete(key []byte) error

	ForEach(func(k, v []byte) error) error

	Scan(prefix []byte, fn func(k, v []byte) error) error
}

// ReadableTx allows one to perform read-only atomic operations on the database.
type ReadableTx interface {
	GetBucket(name []byte) Bucket
}

// WritableTx allows one to perform atomic operations on the database.
type WritableTx interface {
	store.Transaction

	ReadableTx

	GetBucketOrCreate(name []byte) (Bucket, error)
}

// DB is a general interface to operate over a key/value database.
type DB interface {
	// View opens a read-only transaction that will be closed when the closure
	// returns.
	View(fn func(ReadableTx) error) error

	// Update opens a writable transaction that will be committed when the
	// closure returns.
	Update(fn func(WritableTx) error) error

	// Close closes the database and free the resources.
	Close() error
}
