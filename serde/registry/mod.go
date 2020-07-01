package registry

import (
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

type Registry interface {
	Register(serde.Codec, serde.Format)
	Get(serde.Codec) serde.Format
}

type SimpleRegistry struct {
	store map[serde.Codec]serde.Format
}

func NewSimpleRegistry() Registry {
	return &SimpleRegistry{
		store: make(map[serde.Codec]serde.Format),
	}
}

func (r *SimpleRegistry) Register(name serde.Codec, f serde.Format) {
	r.store[name] = f
}

func (r *SimpleRegistry) Get(name serde.Codec) serde.Format {
	fmt := r.store[name]
	if fmt == nil {
		return emptyFormat{name: name}
	}

	return fmt
}

type emptyFormat struct {
	serde.Format
	name serde.Codec
}

func (f emptyFormat) Encode(serde.Context, serde.Message) ([]byte, error) {
	return nil, xerrors.Errorf("format '%s' is not implemented", f.name)
}

func (f emptyFormat) Decode(serde.Context, []byte) (serde.Message, error) {
	return nil, xerrors.Errorf("format '%s' is not implemented", f.name)
}
