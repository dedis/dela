package gob

import (
	"bytes"
	"encoding/gob"

	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

type gobWrapper struct {
	Type  string
	Value []byte
}

type gobDeserializer struct {
	data []byte
}

func (d gobDeserializer) Deserialize(m interface{}) error {
	dec := gob.NewDecoder(bytes.NewBuffer(d.data))
	return dec.Decode(m)
}

// Serializer is a gob serializer.
//
// - implement serde.Serializer
type Serializer struct {
	store serde.Store
}

// NewSerializer returns a gob serializer.
func NewSerializer() serde.Serializer {
	return Serializer{
		store: serde.NewStore(),
	}
}

// GetStore implements serde.Serializer. It returns the factory store.
func (e Serializer) GetStore() serde.Store {
	return e.store
}

// Serialize implements serde.Serializer. It serializes the message using the
// gob format.
func (e Serializer) Serialize(m serde.Message) ([]byte, error) {
	buffer := new(bytes.Buffer)

	itf, err := m.VisitGob()
	if err != nil {
		return nil, err
	}

	enc := gob.NewEncoder(buffer)
	err = enc.Encode(itf)
	if err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

// Deserialize implements serde.Serializer. It returns the message deserialized
// from the data.
func (e Serializer) Deserialize(data []byte, f serde.Factory) (serde.Message, error) {
	m, err := f.VisitGob(gobDeserializer{data: data})
	if err != nil {
		return nil, err
	}

	return m, nil
}

// Wrap implements serde.Serializer. It wraps the message so that a distant peer
// can deserialize it by looking the factory.
func (e Serializer) Wrap(m serde.Message) ([]byte, error) {
	buffer, err := e.Serialize(m)
	if err != nil {
		return nil, err
	}

	msg := gobWrapper{
		Type:  e.store.KeyOf(m),
		Value: buffer,
	}

	msgBuffer := new(bytes.Buffer)
	enc := gob.NewEncoder(msgBuffer)
	err = enc.Encode(msg)
	if err != nil {
		return nil, err
	}

	return msgBuffer.Bytes(), nil
}

// Unwrap implements serde.Serializer. It unwraps a message from its serialized
// data. The factory of the message must be registered beforehands.
func (e Serializer) Unwrap(raw []byte) (serde.Message, error) {
	wrapper := gobWrapper{}

	dec := gob.NewDecoder(bytes.NewBuffer(raw))
	err := dec.Decode(&wrapper)
	if err != nil {
		return nil, err
	}

	factory := e.store.Get(wrapper.Type)
	if factory == nil {
		return nil, xerrors.New("oops")
	}

	m, err := factory.VisitGob(gobDeserializer{data: wrapper.Value})
	if err != nil {
		return nil, err
	}

	return m, nil
}
