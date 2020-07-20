package kv

import (
	"go.etcd.io/bbolt"
	"golang.org/x/xerrors"
)

type boltDB struct {
	bolt *bbolt.DB
}

// New opens a new in-memory database.
func New(path string) (DB, error) {
	db, err := bbolt.Open(path, 0666, &bbolt.Options{})
	if err != nil {
		return nil, err
	}

	bdb := boltDB{
		bolt: db,
	}

	return bdb, nil
}

func (db boltDB) CreateBucket(name []byte) error {
	return db.bolt.Update(func(txn *bbolt.Tx) error {
		_, err := txn.CreateBucketIfNotExists(name)
		return err
	})
}

func (db boltDB) View(bucket []byte, fn func(Bucket) error) error {
	return db.bolt.View(func(txn *bbolt.Tx) error {
		bucket := txn.Bucket(bucket)
		if bucket == nil {
			return xerrors.Errorf("bucket not found")
		}

		return fn(boltBucket{bucket: bucket})
	})
}

func (db boltDB) Update(bucket []byte, fn func(Bucket) error) error {
	return db.bolt.Update(func(txn *bbolt.Tx) error {
		bucket, err := txn.CreateBucketIfNotExists(bucket)
		if err != nil {
			return err
		}

		return fn(boltBucket{bucket: bucket})
	})
}

type boltBucket struct {
	bucket *bbolt.Bucket
}

func (txn boltBucket) Get(key []byte) []byte {
	return txn.bucket.Get(key)
}

func (txn boltBucket) Set(key, value []byte) error {
	return txn.bucket.Put(key, value)
}
