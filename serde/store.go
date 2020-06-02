package serde

import (
	"fmt"
	"reflect"

	"go.dedis.ch/dela"
)

// FactoryStore is a registry of message types. One can register a message
// and retrieve later.
type FactoryStore struct {
	mapper map[string]Factory
}

// NewStore returns a new empty factory store.
func NewStore() Store {
	return &FactoryStore{
		mapper: make(map[string]Factory),
	}
}

// Get implements serde.Store. It returns the factory associated with the given
// key if it exists, otherwise it returns nil.
func (reg *FactoryStore) Get(key string) Factory {
	return reg.mapper[key]
}

// Add adds the factory to the store for the key associated to the message.
func (reg *FactoryStore) Add(m Message, f Factory) error {
	key := reg.KeyOf(m)
	reg.mapper[key] = f

	dela.Logger.Trace().
		Str("key", key).
		Msg("factory registered")

	return nil
}

// KeyOf returns the key associated to a message implementation.
func (reg *FactoryStore) KeyOf(m Message) string {
	typ := reflect.TypeOf(m)
	return fmt.Sprintf("%s.%s", typ.PkgPath(), typ.Name())
}
