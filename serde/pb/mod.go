package pb

import (
	"github.com/golang/protobuf/proto"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=./ ./wrapper.proto

type protoDeserializer struct {
	data []byte
}

func (d protoDeserializer) Deserialize(m interface{}) error {
	pb, ok := m.(proto.Message)
	if !ok {
		return xerrors.New("proto message expected")
	}

	return proto.Unmarshal(d.data, pb)
}

// Serializer is a protobuf serializer.
//
// - implement serde.Serializer
type Serializer struct {
	store serde.Store
}

// NewSerializer returns a new protobuf serializer.
func NewSerializer() serde.Serializer {
	return Serializer{
		store: serde.NewStore(),
	}
}

// GetStore implements serde.Serializer. It returns the factory store.
func (e Serializer) GetStore() serde.Store {
	return e.store
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
	m, err := f.VisitProto(protoDeserializer{data: buffer})
	if err != nil {
		return nil, err
	}

	return m, nil
}

// Wrap implements serde.Serializer. It returns the bytes of a wrapped message
// that a distant party can deserialize by looking up the factory.
func (e Serializer) Wrap(m serde.Message) ([]byte, error) {
	itf, err := m.VisitProto()
	if err != nil {
		return nil, err
	}

	pb, ok := itf.(proto.Message)
	if !ok {
		return nil, xerrors.New("proto message expected")
	}

	value, err := proto.Marshal(pb)
	if err != nil {
		return nil, err
	}

	wrapper := &Wrapper{
		Type:  e.store.KeyOf(m),
		Value: value,
	}

	buffer, err := proto.Marshal(wrapper)
	if err != nil {
		return nil, err
	}

	return buffer, nil
}

// Unwrap implements serde.Serializer. It returns the message implementation of
// the incoming bytes by looking the factory to use to instantiate.
func (e Serializer) Unwrap(buffer []byte) (serde.Message, error) {
	wrapper := &Wrapper{}
	err := proto.Unmarshal(buffer, wrapper)
	if err != nil {
		return nil, err
	}

	factory := e.store.Get(wrapper.Type)
	if factory == nil {
		return nil, xerrors.Errorf("unknown message <%s>", wrapper.Type)
	}

	m, err := factory.VisitProto(protoDeserializer{data: wrapper.GetValue()})
	if err != nil {
		return nil, err
	}

	return m, nil
}
