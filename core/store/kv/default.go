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
func (db boltDB) View(fn func(ReadableTx) error) error {
	return db.bolt.View(func(txn *bbolt.Tx) error {
		return fn(boltTx{txn: txn})
	})
}

// Update implements kv.DB. It opens a read-write transaction and opens the
// bucket. It will create it if it does not exist yet.
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

// BoltTx is the adapter of a bbolt transaction to the kv.Transaction interface.
//
// - implements kv.Transaction
// - implements store.Transaction
type boltTx struct {
	txn *bbolt.Tx
}

// GetBucket implements kv.Transaction. It returns the bucket with the given
// name or nil if it does not exist.
func (tx boltTx) GetBucket(name []byte) Bucket {
	bucket := tx.txn.Bucket(name)
	if bucket == nil {
		return nil
	}

	return boltBucket{bucket: bucket}
}

// GetBucketOrCreate implements kv.Transaction. It creates the bucket if it does
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
