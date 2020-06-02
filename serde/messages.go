package serde

import (
	"fmt"
	"reflect"

	"go.dedis.ch/dela"
)

var defaultRegistry = NewMessageRegistry()

func Register(m interface{}) {
	defaultRegistry.Register(m)
}

func KeyOf(m interface{}) string {
	return defaultRegistry.KeyOf(m)
}

func New(key string) (interface{}, bool) {
	return defaultRegistry.New(key)
}

// MessageRegistry is a registry of message types. One can register a message
// and retrieve later.
type MessageRegistry struct {
	mapper map[string]reflect.Type
}

func NewMessageRegistry() *MessageRegistry {
	return &MessageRegistry{
		mapper: make(map[string]reflect.Type),
	}
}

func (reg *MessageRegistry) Register(m interface{}) {
	key := reg.KeyOf(m)
	reg.mapper[key] = reflect.TypeOf(m)

	dela.Logger.Trace().Msgf("message <%s> registered", key)
}

func (reg *MessageRegistry) KeyOf(m interface{}) string {
	typ := reflect.TypeOf(m)
	return fmt.Sprintf("%s.%s", typ.PkgPath(), typ.Name())
}

func (reg *MessageRegistry) New(key string) (interface{}, bool) {
	typ := reg.mapper[key]
	if typ == nil {
		return nil, false
	}

	return reflect.New(typ).Interface(), true
}
