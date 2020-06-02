package pb

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/serde"
)

func TestSerializer_Serialize(t *testing.T) {
	s := NewSerializer()

	b := block{Value: "Hello World!"}

	buffer, err := s.Serialize(b)
	require.NoError(t, err)

	ret := &wrappers.StringValue{}
	err = proto.Unmarshal(buffer, ret)
	require.NoError(t, err)
	require.Equal(t, &wrappers.StringValue{Value: b.Value}, ret)
}

func TestSerializer_Deserialize(t *testing.T) {
	s := NewSerializer()

	b := block{Value: "Hello World!"}

	buffer, err := proto.Marshal(&wrappers.StringValue{Value: b.Value})
	require.NoError(t, err)

	m, err := s.Deserialize(buffer, blockFactory{})
	require.NoError(t, err)
	require.Equal(t, b, m)
}

func TestSerializer_Wrap(t *testing.T) {
	s := NewSerializer()

	b := block{Value: "Hello World!"}

	buffer, err := s.Wrap(b)
	require.NoError(t, err)

	wrapper := &Wrapper{}
	err = proto.Unmarshal(buffer, wrapper)
	require.NoError(t, err)
	require.Equal(t, s.GetStore().KeyOf(b), wrapper.GetType())

	value := &wrappers.StringValue{}
	err = proto.Unmarshal(wrapper.GetValue(), value)
	require.NoError(t, err)
	require.Equal(t, b, block{Value: value.GetValue()})
}

func TestDeserializer_Unwrap(t *testing.T) {
	s := NewSerializer()
	require.NoError(t, s.GetStore().Add(block{}, blockFactory{}))

	b := block{Value: "Hello World!"}

	value, err := proto.Marshal(&wrappers.StringValue{Value: b.Value})
	require.NoError(t, err)

	buffer, err := proto.Marshal(&Wrapper{
		Type:  s.GetStore().KeyOf(b),
		Value: value,
	})
	require.NoError(t, err)

	m, err := s.Unwrap(buffer)
	require.NoError(t, err)
	require.Equal(t, b, m)
}

// -----------------------------------------------------------------------------
// Utility functions

type block struct {
	serde.UnimplementedMessage

	Value string
}

func (m block) VisitProto() (interface{}, error) {
	t := &wrappers.StringValue{Value: m.Value}

	return t, nil
}

type blockFactory struct {
	serde.UnimplementedFactory
}

func (f blockFactory) VisitProto(d serde.Deserializer) (serde.Message, error) {
	m := &wrappers.StringValue{}
	err := d.Deserialize(m)
	if err != nil {
		return nil, err
	}

	t := block{
		Value: m.GetValue(),
	}

	return t, nil
}
