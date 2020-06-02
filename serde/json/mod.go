package encoder

import (
	"encoding/json"

	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// jsonWrapper is the wrapper to allow a message to decode by itself.
type jsonWrapper struct {
	Type  string
	Value json.RawMessage
}

type jsonDeserializer struct {
	data []byte
}

func (d jsonDeserializer) Deserialize(m interface{}) error {
	return json.Unmarshal(d.data, m)
}

// Serializer is an encoder using JSON as the underlying format.
type Serializer struct {
	store serde.Store
}

// NewSerializer returns a new JSON serializer.
func NewSerializer() serde.Serializer {
	return Serializer{
		store: serde.NewStore(),
	}
}

// GetStore implements serde.Serializer.
func (e Serializer) GetStore() serde.Store {
	return e.store
}

// Serialize implements serde.Serializer.
func (e Serializer) Serialize(m serde.Message) ([]byte, error) {
	itf, err := m.VisitJSON()
	if err != nil {
		return nil, err
	}
	return json.Marshal(itf)
}

// Deserialize implements serde.Deserialize.
func (e Serializer) Deserialize(buffer []byte, f serde.Factory) (serde.Message, error) {
	m, err := f.VisitJSON(jsonDeserializer{data: buffer})
	if err != nil {
		return nil, err
	}

	return m, nil
}

// Wrap implements serde.Wrap.
func (e Serializer) Wrap(m serde.Message) ([]byte, error) {
	itf, err := m.VisitJSON()
	if err != nil {
		return nil, err
	}

	buffer, err := json.Marshal(itf)
	if err != nil {
		return nil, err
	}

	typ := e.store.KeyOf(m)

	msg := jsonWrapper{
		Type:  typ,
		Value: buffer,
	}

	msgBuffer, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	return msgBuffer, nil
}

// Unwrap implements serde.Serializer.
func (e Serializer) Unwrap(data []byte) (serde.Message, error) {
	wrapper := jsonWrapper{}
	err := json.Unmarshal(data, &wrapper)
	if err != nil {
		return nil, err
	}

	factory := e.store.Get(wrapper.Type)
	if factory == nil {
		return nil, xerrors.New("oops")
	}

	m, err := factory.VisitJSON(jsonDeserializer{data: wrapper.Value})
	if err != nil {
		return nil, err
	}

	return m, nil
}
