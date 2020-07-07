package inmemory

import (
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// NewInMemory returns a new in memory database
func NewInMemory() *InMemory {
	return &InMemory{
		database: make(map[string]serde.Message),
	}
}

// InMemory implements an in memory key value storage
//
// implements storage.KeyValue
type InMemory struct {
	database map[string]serde.Message
}

// Store implements storage.KeyValue
func (i *InMemory) Store(key []byte, value serde.Message) error {
	i.database[string(key)] = value

	return nil
}

// Read implements storage.Read
func (i *InMemory) Read(key []byte) (serde.Message, error) {
	res, found := i.database[string(key)]
	if !found {
		return nil, xerrors.New("key not found")
	}

	return res, nil
}
