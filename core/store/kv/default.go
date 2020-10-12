// This file contains the implementation of a key/value database using bbolt.
//
// See https://github.com/etcd-io/bbolt.
//
// Documentation Last Review: 08.10.2020
//

package kv

import (
	"bytes"

	"go.etcd.io/bbolt"
	"golang.org/x/xerrors"
)

// BoltDB is an adapter of the KV database using bboltdb.
//
// - implements kv.DB
type boltDB struct {
	bolt *bbolt.DB
}

// New opens a new database to the given file.
func New(path string) (DB, error) {
	db, err := bbolt.Open(path, 0666, &bbolt.Options{})
	if err != nil {
		return nil, xerrors.Errorf("failed to open db: %v", err)
	}

	bdb := boltDB{
		bolt: db,
	}

	return bdb, nil
}

// View implements kv.DB. It executes the read-only transaction in the context
// of the database.
func (db boltDB) View(fn func(ReadableTx) error) error {
	return db.bolt.View(func(txn *bbolt.Tx) error {
		return fn(boltTx{txn: txn})
	})
}

// Update implements kv.DB. It executes the writable transaction in the context
// of the database.
func (db boltDB) Update(fn func(WritableTx) error) error {
	return db.bolt.Update(func(txn *bbolt.Tx) error {
		return fn(boltTx{txn: txn})
	})
}

// Close implements kv.DB. It closes the database. Any view or update call will
// result in an error after this function is called.
func (db boltDB) Close() error {
	return db.bolt.Close()
}

// BoltTx is the adapter of a bbolt transaction for the key/value database.
//
// - implements kv.ReadableTx
// - implements kv.WritableTx
type boltTx struct {
	txn *bbolt.Tx
}

// GetBucket implements kv.ReadableTx. It returns the bucket with the given name
// or nil if it does not exist.
func (tx boltTx) GetBucket(name []byte) Bucket {
	bucket := tx.txn.Bucket(name)
	if bucket == nil {
		return nil
	}

	return boltBucket{bucket: bucket}
}

// GetBucketOrCreate implements kv.WritableTx. It creates the bucket if it does
// not exist and then return it.
func (tx boltTx) GetBucketOrCreate(name []byte) (Bucket, error) {
	bucket, err := tx.txn.CreateBucketIfNotExists(name)
	if err != nil {
		return nil, xerrors.Errorf("create bucket failed: %v", err)
	}

	return boltBucket{bucket: bucket}, nil
}

// OnCommit implements store.Transaction. It registers a callback that is called
// after the transaction is successful.
func (tx boltTx) OnCommit(fn func()) {
	tx.txn.OnCommit(fn)
}

// BoltBucket is the adapter of a bbolt bucket for the key/value database.
//
// - implements kv.Bucket
type boltBucket struct {
	bucket *bbolt.Bucket
}

// Get implements kv.Bucket. It returns the value associated to the key, or nil
// if it does not exist.
func (txn boltBucket) Get(key []byte) []byte {
	return txn.bucket.Get(key)
}

// Set implements kv.Bucket. It sets the provided key to the value.
func (txn boltBucket) Set(key, value []byte) error {
	return txn.bucket.Put(key, value)
}

// Delete implements kv.Bucket. It deletes the key from the bucket.
func (txn boltBucket) Delete(key []byte) error {
	return txn.bucket.Delete(key)
}

// ForEach implements kv.Bucket. It iterates over the whole bucket in an
// unspecified order. If the callback returns an error, the iteration is stopped
// and the error returned to the caller.
func (txn boltBucket) ForEach(fn func(k, v []byte) error) error {
	return txn.bucket.ForEach(fn)
}

// Scan implements kv.Bucket. It iterates over the keys matching the prefix in a
// sorted order. If the callback returns an error, the iteration is stopped and
// the error returned to the caller.
func (txn boltBucket) Scan(prefix []byte, fn func(k, v []byte) error) error {
	cursor := txn.bucket.Cursor()
	cursor.Seek(prefix)

	for k, v := cursor.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = cursor.Next() {
		err := fn(k, v)
		if err != nil {
			// The caller is responsible for wrapping the errors inside the
			// callback, as it returns the exact error to allow comparison.
			return err
		}
	}

	return nil
}
