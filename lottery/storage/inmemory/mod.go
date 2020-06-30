package inmemory

import "golang.org/x/xerrors"

// NewInMemory returns a new in memory database
func NewInMemory() *InMemory {
	return &InMemory{
		database: make(map[string][]byte),
	}
}

// InMemory implements an in memory key value storage
//
// implements storage.KeyValue
type InMemory struct {
	database map[string][]byte
}

// Store implements storage.KeyValue
func (i *InMemory) Store(key, value []byte) error {
	i.database[string(key)] = value
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
