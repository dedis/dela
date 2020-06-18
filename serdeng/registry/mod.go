package registry

import (
	"go.dedis.ch/dela/serdeng"
	"golang.org/x/xerrors"
)

type Registry interface {
	Register(serdeng.Codec, serdeng.Format)
	Get(serdeng.Codec) serdeng.Format
}

type SimpleRegistry struct {
	store map[serdeng.Codec]serdeng.Format
}

func NewSimpleRegistry() Registry {
	return &SimpleRegistry{
		store: make(map[serdeng.Codec]serdeng.Format),
	}
}

func (r *SimpleRegistry) Register(name serdeng.Codec, f serdeng.Format) {
	r.store[name] = f
}

func (r *SimpleRegistry) Get(name serdeng.Codec) serdeng.Format {
	fmt := r.store[name]
	if fmt == nil {
		return emptyFormat{name: name}
	}

	return fmt
}

type emptyFormat struct {
	serdeng.Format
	name serdeng.Codec
}

func (f emptyFormat) Encode(serdeng.Context, serdeng.Message) ([]byte, error) {
	return nil, xerrors.Errorf("format '%s' is not implemented", f.name)
}

func (f emptyFormat) Decode(serdeng.Context, []byte) (serdeng.Message, error) {
	return nil, xerrors.Errorf("format '%s' is not implemented", f.name)
}
