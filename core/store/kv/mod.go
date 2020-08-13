package kv

// Bucket is a general interface to operate on a database bucket.
type Bucket interface {
	Get(key []byte) []byte

	Set(key, value []byte) error

	Delete(key []byte) error

	ForEach(func(k, v []byte) error) error

	Scan(prefix []byte, fn func(k, v []byte) error) error
}

// DB is a general interface to operate over a key/value database.
type DB interface {
	View(bucket []byte, fn func(Bucket) error) error

	Update(bucket []byte, fn func(Bucket) error) error

	Close() error
}
