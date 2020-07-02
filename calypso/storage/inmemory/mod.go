package inmemory

import (
	"go.dedis.ch/dela/serde"
	serdej "go.dedis.ch/dela/serde/json"
	"golang.org/x/xerrors"
)

// NewInMemory returns a new in memory database
func NewInMemory() *InMemory {
	return &InMemory{
		database:   make(map[string][]byte),
		serializer: serdej.NewSerializer(),
	}
}

// InMemory implements an in memory key value storage
//
// implements storage.KeyValue
type InMemory struct {
	database   map[string][]byte
	serializer serde.Serializer
}

// Store implements storage.KeyValue
func (i *InMemory) Store(key []byte, value serde.Message) error {
	buf, err := i.serializer.Serialize(value)
	if err != nil {
		return xerrors.Errorf("failed to serialize: %v", err)
	}

	i.database[string(key)] = buf

	return nil
}

// Read implements storage.Read
func (i *InMemory) Read(key []byte) ([]byte, error) {
	res, found := i.database[string(key)]
	if !found {
		return nil, xerrors.New("key not found")
	}

	return res, nil
}
