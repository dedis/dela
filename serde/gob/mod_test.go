package gob

import (
	"bytes"
	"encoding/gob"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/serde"
)

func TestSerializer_Serialize(t *testing.T) {
	s := NewSerializer()

	b := block{
		Index: 42,
		Value: "Hello World!",
	}

	buffer, err := s.Serialize(b)
	require.NoError(t, err)

	dec := gob.NewDecoder(bytes.NewBuffer(buffer))
	ret := blockMessage{}
	err = dec.Decode(&ret)
	require.NoError(t, err)
	require.Equal(t, blockMessage{Index: b.Index, Value: b.Value}, ret)
}

func TestSerializer_Deserialize(t *testing.T) {
	s := NewSerializer()

	b := blockMessage{Index: 42, Value: "Hello World!"}
	out := new(bytes.Buffer)
	enc := gob.NewEncoder(out)
	err := enc.Encode(b)
	require.NoError(t, err)

	m, err := s.Deserialize(out.Bytes(), blockFactory{})
	require.NoError(t, err)
	require.Equal(t, block{Index: b.Index, Value: b.Value}, m)
}

func TestSerializer_Wrap(t *testing.T) {
	s := NewSerializer()

	b := block{Index: 42, Value: "Hello World!"}

	buffer, err := s.Wrap(b)
	require.NoError(t, err)

	dec := gob.NewDecoder(bytes.NewBuffer(buffer))
	wrapper := gobWrapper{}
	err = dec.Decode(&wrapper)
	require.NoError(t, err)

	require.Equal(t, "go.dedis.ch/dela/serde/gob.block", wrapper.Type)

	dec = gob.NewDecoder(bytes.NewBuffer(wrapper.Value))
	m := blockMessage{}
	err = dec.Decode(&m)
	require.NoError(t, err)
	require.Equal(t, blockMessage{Index: b.Index, Value: b.Value}, m)
}

func TestSerializer_Unwrap(t *testing.T) {
	s := NewSerializer()
	require.NoError(t, s.GetStore().Add(block{}, blockFactory{}))

	b := block{
		Index: 42,
		Value: "Hello World!",
	}

	value := marshal(t, blockMessage{Index: b.Index, Value: b.Value})
	wrapper := marshal(t, gobWrapper{
		Type:  s.GetStore().KeyOf(b),
		Value: value,
	})

	m, err := s.Unwrap(wrapper)
	require.NoError(t, err)
	require.Equal(t, b, m)
}

// -----------------------------------------------------------------------------
// Utility functions

func marshal(t *testing.T, m interface{}) []byte {
	out := new(bytes.Buffer)
	enc := gob.NewEncoder(out)
	err := enc.Encode(m)
	require.NoError(t, err)

	return out.Bytes()
}

type blockMessage struct {
	Index uint64
	Value string
}

type block struct {
	serde.UnimplementedMessage

	Index uint64
	Value string
}

func (m block) VisitGob() (interface{}, error) {
	t := blockMessage{
		Value: m.Value,
		Index: m.Index,
	}

	return t, nil
}

type blockFactory struct {
	serde.UnimplementedFactory
}

func (f blockFactory) VisitGob(d serde.Deserializer) (serde.Message, error) {
	m := blockMessage{}
	err := d.Deserialize(&m)
	if err != nil {
		return nil, err
	}

	t := block{
		Value: m.Value,
		Index: m.Index,
	}

	return t, nil
}
