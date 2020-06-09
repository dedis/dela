package json

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func TestFactoryInput_GetSerializer(t *testing.T) {
	input := factoryInput{
		serde: NewSerializer(),
	}

	require.NotNil(t, input.GetSerializer())
}

func TestFactoryInput_Feed(t *testing.T) {
	input := factoryInput{
		data: []byte("{}"),
	}

	ret := struct{}{}
	err := input.Feed(&ret)
	require.NoError(t, err)

	err = input.Feed(nil)
	require.EqualError(t, err, "couldn't unmarshal: json: Unmarshal(nil)")
}

func TestJsonEncoder_Serialize(t *testing.T) {
	s := NewSerializer()

	dm := block{
		Value: "Hello World!",
		Index: 42,
	}

	buffer, err := s.Serialize(dm)
	require.NoError(t, err)
	require.Equal(t, "{\"Index\":42,\"Value\":\"Hello World!\"}", string(buffer))

	_, err = s.Serialize(badMessage{err: xerrors.New("oops")})
	require.EqualError(t, err,
		"couldn't serialize 'json.badMessage' to json: oops")

	_, err = s.Serialize(badMessage{})
	require.EqualError(t, err,
		"couldn't encode: json: error calling MarshalJSON for type json.badJSON: oops")
}

func TestSerializer_Deserialize(t *testing.T) {
	s := NewSerializer()

	buffer := []byte("{\"Value\":\"Hello World!\",\"Index\":42}")

	var m block
	err := s.Deserialize(buffer, blockFactory{}, &m)
	require.NoError(t, err)
	require.Equal(t, block{Value: "Hello World!", Index: 42}, m)

	err = s.Deserialize(buffer, badFactory{}, &m)
	require.EqualError(t, err,
		"couldn't deserialize from json with 'json.badFactory': oops")

	err = s.Deserialize(buffer, blockFactory{}, nil)
	require.EqualError(t, err, "couldn't assign: expect a pointer")
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

func (m block) VisitJSON(serde.Serializer) (interface{}, error) {
	t := blockMessage{
		Value: m.Value,
		Index: m.Index,
	}

	return t, nil
}

type blockFactory struct {
	serde.UnimplementedFactory
}

func (f blockFactory) VisitJSON(input serde.FactoryInput) (serde.Message, error) {
	m := blockMessage{}
	err := input.Feed(&m)
	if err != nil {
		return nil, err
	}

	t := block{
		Value: m.Value,
		Index: m.Index,
	}

	return t, nil
}

type badJSON struct {
	json.Marshaler
}

func (j badJSON) MarshalJSON() ([]byte, error) {
	return nil, xerrors.New("oops")
}

type badMessage struct {
	serde.UnimplementedMessage

	err error
}

func (m badMessage) VisitJSON(serde.Serializer) (interface{}, error) {
	return badJSON{}, m.err
}

type badFactory struct {
	serde.UnimplementedFactory
}

func (m badFactory) VisitJSON(serde.FactoryInput) (serde.Message, error) {
	return nil, xerrors.New("oops")
}
