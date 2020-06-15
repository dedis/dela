package gob

import (
	"bytes"
	"encoding/gob"
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
	data := marshal(t, blockMessage{})

	input := factoryInput{data: data}

	ret := blockMessage{}
	err := input.Feed(&ret)
	require.NoError(t, err)
	require.Equal(t, blockMessage{}, ret)

	input = factoryInput{}
	err = input.Feed(&ret)
	require.EqualError(t, err, "couldn't decode to interface: EOF")
}

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

	_, err = s.Serialize(badMessage{err: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't visit message: oops")

	_, err = s.Serialize(badMessage{})
	require.EqualError(t, err, "couldn't encode: gob: cannot encode nil value")
}

func TestSerializer_Deserialize(t *testing.T) {
	s := NewSerializer()

	b := blockMessage{Index: 42, Value: "Hello World!"}
	out := new(bytes.Buffer)
	enc := gob.NewEncoder(out)
	err := enc.Encode(b)
	require.NoError(t, err)

	var m block
	err = s.Deserialize(out.Bytes(), blockFactory{}, &m)
	require.NoError(t, err)
	require.Equal(t, block{Index: b.Index, Value: b.Value}, m)

	err = s.Deserialize(out.Bytes(), badFactory{}, &m)
	require.EqualError(t, err, "couldn't visit factory: oops")

	err = s.Deserialize(out.Bytes(), blockFactory{}, nil)
	require.EqualError(t, err, "couldn't assign: expect a pointer")
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

func (m block) VisitGob(serde.Serializer) (interface{}, error) {
	t := blockMessage{
		Value: m.Value,
		Index: m.Index,
	}

	return t, nil
}

type blockFactory struct {
	serde.UnimplementedFactory
}

func (f blockFactory) VisitGob(input serde.FactoryInput) (serde.Message, error) {
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

type badMessage struct {
	serde.UnimplementedMessage

	err error
}

func (m badMessage) VisitGob(serde.Serializer) (interface{}, error) {
	return nil, m.err
}

type badFactory struct {
	serde.UnimplementedFactory
}

func (m badFactory) VisitGob(serde.FactoryInput) (serde.Message, error) {
	return nil, xerrors.New("oops")
}
