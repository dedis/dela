// Package json implements a JSON serializer. Please refer to the official
// documentation to learn about the specificity of the format.
//
// https://golang.org/pkg/encoding/json/
package json

import (
	"encoding/json"

	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// FactoryInput is an implementation of the factory input.
//
// - implement serde.FactoryInput
type factoryInput struct {
	data []byte
}

// Feed implements serde.FactoryInput. It decodes the data into the given
// interface.
func (d factoryInput) Feed(m interface{}) error {
	err := json.Unmarshal(d.data, m)
	if err != nil {
		return xerrors.Errorf("couldn't unmarshal: %v", err)
	}

	return nil
}

// Serializer is an encoder using JSON as the underlying format.
type Serializer struct{}

// NewSerializer returns a new JSON serializer.
func NewSerializer() serde.Serializer {
	return Serializer{}
}

// Serialize implements serde.Serializer.
func (e Serializer) Serialize(m serde.Message) ([]byte, error) {
	itf, err := m.VisitJSON()
	if err != nil {
		return nil, xerrors.Errorf("couldn't visit message: %v", err)
	}

	buffer, err := json.Marshal(itf)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode: %v", err)
	}

	return buffer, nil
}

// Deserialize implements serde.Deserialize.
func (e Serializer) Deserialize(buffer []byte, f serde.Factory) (serde.Message, error) {
	m, err := f.VisitJSON(factoryInput{data: buffer})
	if err != nil {
		return nil, xerrors.Errorf("couldn't visit factory: %v", err)
	}

	return m, nil
}
