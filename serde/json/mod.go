// Package json implements a JSON serializer. Please refer to the official
// documentation to learn about the specificity of the format.
//
// https://golang.org/pkg/encoding/json/
package json

import (
	"encoding/json"

	"go.dedis.ch/dela/internal/serdereflect"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// FactoryInput is an implementation of the factory input.
//
// - implements serde.FactoryInput
type factoryInput struct {
	serde serde.Serializer
	data  []byte
}

// GetSerializer implements serde.FactoryInput. It returns the serializer of the
// context.
func (d factoryInput) GetSerializer() serde.Serializer {
	return d.serde
}

// Feed implements serde.FactoryInput. It decodes the data into the given
// interface.
func (d factoryInput) Feed(m interface{}) error {
	err := json.Unmarshal(d.data, m)
	if err != nil {
		return xerrors.Errorf("couldn't unmarshal: %w", err)
	}

	return nil
}

// Serializer is an encoder using JSON as the underlying format.
//
// - implements serde.Serializer
type Serializer struct{}

// NewSerializer returns a new JSON serializer.
func NewSerializer() serde.Serializer {
	return Serializer{}
}

// Serialize implements serde.Serializer.
func (e Serializer) Serialize(m serde.Message) ([]byte, error) {
	if m == nil {
		return nil, xerrors.New("message is nil")
	}

	itf, err := m.VisitJSON(e)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize '%T' to json: %w", m, err)
	}

	buffer, err := json.Marshal(itf)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode: %v", err)
	}

	return buffer, nil
}

// Deserialize implements serde.Deserialize.
func (e Serializer) Deserialize(buffer []byte, f serde.Factory, o interface{}) error {
	if f == nil {
		return xerrors.New("factory is nil")
	}

	m, err := f.VisitJSON(factoryInput{data: buffer, serde: e})
	if err != nil {
		return xerrors.Errorf("couldn't deserialize from json with '%T': %w", f, err)
	}

	err = serdereflect.AssignTo(m, o)
	if err != nil {
		return xerrors.Errorf("couldn't assign: %v", err)
	}

	return nil
}
