package kv

type Bucket interface {
	Get(key []byte) []byte
	Set(key, value []byte) error
	ForEach(func(k, v []byte) error) error
}

type DB interface {
	CreateBucket(name []byte) error
	View(bucket []byte, fn func(Bucket) error) error
	Update(bucket []byte, fn func(Bucket) error) error
}
