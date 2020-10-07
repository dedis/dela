// This file contains the implementation of a format registry.
//
// Documentation Last Review: 07.10.2020
//

package registry

import (
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// SimpleRegistry is a default implementation of the Registry interface. It will
// always return a format which means an empty one is returned if the key is
// unknown.
//
// - implements registry.Registry
type SimpleRegistry struct {
	store map[serde.Format]serde.FormatEngine
}

// NewSimpleRegistry returns a new empty registry.
func NewSimpleRegistry() *SimpleRegistry {
	return &SimpleRegistry{
		store: make(map[serde.Format]serde.FormatEngine),
	}
}

// Register implements registry.Registry. It registers the engine for the given
// format.
func (r *SimpleRegistry) Register(name serde.Format, f serde.FormatEngine) {
	r.store[name] = f
}

// Get implements registry.Registry. It returns the format engine associated
// with the format if it exists, otherwise it returns an empty format.
func (r *SimpleRegistry) Get(name serde.Format) serde.FormatEngine {
	fmt := r.store[name]
	if fmt == nil {
		return emptyFormat{name: name}
	}

	return fmt
}

// EmptyFormat is an implementation of the FormatEngine interface. It implements
// the functions but always returns an error so that the serialization and
// deserialization can fail with meaningful errors without checking the format
// existance.
//
// - implements serde.FormatEngine
type emptyFormat struct {
	serde.FormatEngine
	name serde.Format
}

// Encode implements serde.FormatEngine. It always returns an error.
func (f emptyFormat) Encode(serde.Context, serde.Message) ([]byte, error) {
	return nil, xerrors.Errorf("format '%s' is not implemented", f.name)
}

// Decode implements serde.FormatEngine. It always returns an error.
func (f emptyFormat) Decode(serde.Context, []byte) (serde.Message, error) {
	return nil, xerrors.Errorf("format '%s' is not implemented", f.name)
}
