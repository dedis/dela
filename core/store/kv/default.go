package kv

import (
	"bytes"

	"go.etcd.io/bbolt"
	"golang.org/x/xerrors"
)

// BoltDB is an adapter of the KV store using bboltdb.
//
// - implements kv.DB
type boltDB struct {
	bolt *bbolt.DB
}

// New opens a new in-memory database.
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

// View implements kv.DB. It opens a read-only transaction and opens the
// provided bucket. It will return an error if the bucket does not exist.
func (db boltDB) View(bucket []byte, fn func(Bucket) error) error {
	return db.bolt.View(func(txn *bbolt.Tx) error {
		b := txn.Bucket(bucket)
		if b == nil {
			return xerrors.Errorf("bucket '%x' not found", bucket)
		}

		return fn(boltBucket{bucket: b})
	})
}

// Update implements kv.DB. It opens a read-write transaction and opens the
// bucket. It will create it if it does not exist yet.
func (db boltDB) Update(bucket []byte, fn func(Bucket) error) error {
	return db.bolt.Update(func(txn *bbolt.Tx) error {
		bucket, err := txn.CreateBucketIfNotExists(bucket)
		if err != nil {
			return xerrors.Errorf("failed to create bucket: %v", err)
		}

		return fn(boltBucket{bucket: bucket})
	})
}

// Close implements kv.DB. It closes the database. Any view or update call will
// result in an error after this function is called.
func (db boltDB) Close() error {
	return db.bolt.Close()
}

// BoltBucket is the adapter of a bbolt bucket to the kv.Bucket interface.
//
// - implements kv.Bucket
type boltBucket struct {
	bucket *bbolt.Bucket
}

// Get implements kv.Bucket. It returns the value associated to the key.
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

// ForEach implements kv.Bucket. It iterates over the whole bucket.
func (txn boltBucket) ForEach(fn func(k, v []byte) error) error {
	return txn.bucket.ForEach(fn)
}

// Scan implements kv.Bucket. It iterates over the keys matching the prefix.
func (txn boltBucket) Scan(prefix []byte, fn func(k, v []byte) error) error {
	cursor := txn.bucket.Cursor()
	cursor.Seek(prefix)

	for k, v := cursor.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = cursor.Next() {
		err := fn(k, v)
		if err != nil {
			return xerrors.Errorf("callback failed: %v", err)
		}
	}

	return nil
}
