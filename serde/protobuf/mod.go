// Package protobuf implements a Protobuf serializer. Please refer to the
// official documentation to learn about the specificity of the format.
//
// https://pkg.go.dev/github.com/golang/protobuf
package protobuf

import (
	"github.com/golang/protobuf/proto"
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

// GetSerializer implements serde.FactoryInput. It returns the serializer for
// the context.
func (d factoryInput) GetSerializer() serde.Serializer {
	return d.serde
}

// Feed implements serde.FactoryInput. It decodes the data into the given
// interface.
func (d factoryInput) Feed(m interface{}) error {
	pb, ok := m.(proto.Message)
	if !ok {
		return xerrors.Errorf("invalid message type '%T'", m)
	}

	err := proto.Unmarshal(d.data, pb)
	if err != nil {
		return xerrors.Errorf("couldn't unmarshal: %v", err)
	}

	return nil
}

// Serializer is a protobuf serializer.
//
// - implements serde.Serializer
type Serializer struct{}

// NewSerializer returns a new protobuf serializer.
func NewSerializer() serde.Serializer {
	return Serializer{}
}

// Serialize implements serde.Serializer. It returns the bytes for the message
// implementation using protobuf.
func (e Serializer) Serialize(m serde.Message) ([]byte, error) {
	itf, err := m.VisitProto(e)
	if err != nil {
		return nil, xerrors.Errorf("couldn't visit message: %v", err)
	}

	pb, ok := itf.(proto.Message)
	if !ok {
		return nil, xerrors.New("visit should return a proto message")
	}

	buffer, err := proto.Marshal(pb)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal: %v", err)
	}

	return buffer, nil
}

// Deserialize implements serde.Serializer. It returns the message associated to
// the bytes using protobuf.
func (e Serializer) Deserialize(buffer []byte, f serde.Factory, o interface{}) error {
	m, err := f.VisitProto(factoryInput{data: buffer, serde: e})
	if err != nil {
		return xerrors.Errorf("couldn't visit factory: %v", err)
	}

	err = serdereflect.AssignTo(m, o)
	if err != nil {
		return xerrors.Errorf("couldn't assign: %v", err)
	}

	return nil
}
