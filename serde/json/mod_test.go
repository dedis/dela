package encoder

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/serde"
)

func TestJsonEncoder_Serialize(t *testing.T) {
	s := NewSerializer()

	dm := block{
		Value: "Hello World!",
		Index: 42,
	}

	buffer, err := s.Serialize(dm)
	require.NoError(t, err)
	require.Equal(t, "{\"Index\":42,\"Value\":\"Hello World!\"}", string(buffer))
}

func TestSerializer_Deserialize(t *testing.T) {
	s := NewSerializer()

	buffer := []byte("{\"Value\":\"Hello World!\",\"Index\":42}")

	m, err := s.Deserialize(buffer, blockFactory{})
	require.NoError(t, err)
	require.Equal(t, block{Value: "Hello World!", Index: 42}, m)
}

func TestSerializer_Wrap(t *testing.T) {
	s := NewSerializer()

	b := block{
		Index: 0,
		Value: "deadbeef",
	}

	buffer, err := s.Wrap(b)
	require.NoError(t, err)
	require.Equal(t, "{\"Type\":\"go.dedis.ch/dela/serde/json.block\",\"Value\":{\"Index\":0,\"Value\":\"deadbeef\"}}", string(buffer))
}

func TestSerializer_Unwrap(t *testing.T) {
	s := NewSerializer()
	require.NoError(t, s.GetStore().Add(block{}, blockFactory{}))

	buffer := []byte("{\"Type\":\"go.dedis.ch/dela/serde/json.block\",\"Value\":{\"Index\":0,\"Value\":\"deadbeef\"}}")

	m, err := s.Unwrap(buffer)
	require.NoError(t, err)
	require.Equal(t, block{Index: 0, Value: "deadbeef"}, m)
}

// -----------------------------------------------------------------------------
// Utility functions

type blockMessage struct {
	Index uint64
	Value string
}

type block struct {
	serde.UnimplementedMessage

	Index uint64
	Value string
}

func (m block) VisitJSON() (interface{}, error) {
	t := blockMessage{
		Value: m.Value,
		Index: m.Index,
	}

	return t, nil
}

type blockFactory struct {
	serde.UnimplementedFactory
}

func (f blockFactory) VisitJSON(d serde.Deserializer) (serde.Message, error) {
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
