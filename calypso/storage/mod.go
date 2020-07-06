package storage

import "go.dedis.ch/dela/serde"

// KeyValue defines a simple key value storage
type KeyValue interface {
	Store(key []byte, value serde.Message) error
	Read(key []byte) (serde.Message, error)
}
