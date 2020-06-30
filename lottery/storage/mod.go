package storage

// KeyValue defines a simple key value storage
type KeyValue interface {
	Store(key, value []byte) error
	Read(key []byte) ([]byte, error)
}
