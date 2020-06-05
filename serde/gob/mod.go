// Package gob implements a serializer that uses the gob format. Please refer to
// the official documentation to learn about the specificity of the format.
//
// https://golang.org/pkg/encoding/gob/
package gob

import (
	"bytes"
	"encoding/gob"

	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// FactoryInput is an implementation of the factory input.
//
// - implements serde.FactoryInput
type factoryInput struct {
	data []byte
}

// Feed implements serde.FactoryInput. It decodes the data into the given
// interface.
func (d factoryInput) Feed(m interface{}) error {
	dec := gob.NewDecoder(bytes.NewBuffer(d.data))
	err := dec.Decode(m)
	if err != nil {
		return xerrors.Errorf("couldn't decode to interface: %v", err)
	}

	return nil
}

// Serializer is a gob serializer.
//
// - implements serde.Serializer
type Serializer struct{}

// NewSerializer returns a gob serializer.
func NewSerializer() serde.Serializer {
	return Serializer{}
}

// Serialize implements serde.Serializer. It serializes the message using the
// gob format.
func (e Serializer) Serialize(m serde.Message) ([]byte, error) {
	itf, err := m.VisitGob()
	if err != nil {
		return nil, xerrors.Errorf("couldn't visit message: %v", err)
	}

	buffer := new(bytes.Buffer)
	enc := gob.NewEncoder(buffer)

	err = enc.Encode(itf)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode: %v", err)
	}

	return buffer.Bytes(), nil
}

// Deserialize implements serde.Serializer. It returns the message deserialized
// from the data.
func (e Serializer) Deserialize(data []byte, f serde.Factory) (serde.Message, error) {
	m, err := f.VisitGob(factoryInput{data: data})
	if err != nil {
		return nil, xerrors.Errorf("couldn't visit factory: %v", err)
	}

	return m, nil
}
