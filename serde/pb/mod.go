package pb

import (
	"github.com/golang/protobuf/proto"
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
	pb, ok := m.(proto.Message)
	if !ok {
		return xerrors.New("proto message expected")
	}

	return proto.Unmarshal(d.data, pb)
}

// Serializer is a protobuf serializer.
//
// - implement serde.Serializer
type Serializer struct{}

// NewSerializer returns a new protobuf serializer.
func NewSerializer() serde.Serializer {
	return Serializer{}
}

// Serialize implements serde.Serializer. It returns the bytes for the message
// implementation using protobuf.
func (e Serializer) Serialize(m serde.Message) ([]byte, error) {
	itf, err := m.VisitProto()
	if err != nil {
		return nil, err
	}

	pb, ok := itf.(proto.Message)
	if !ok {
		return nil, xerrors.New("visitor should return a proto message")
	}

	buffer, err := proto.Marshal(pb)
	if err != nil {
		return nil, err
	}

	return buffer, nil
}

// Deserialize implements serde.Serializer. It returns the message associated to
// the bytes using protobuf.
func (e Serializer) Deserialize(buffer []byte, f serde.Factory) (serde.Message, error) {
	m, err := f.VisitProto(factoryInput{data: buffer})
	if err != nil {
		return nil, err
	}

	return m, nil
}
