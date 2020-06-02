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

func (f blockFactory) VisitProto(input serde.FactoryInput) (serde.Message, error) {
	m := &wrappers.StringValue{}
	err := input.Feed(m)
	if err != nil {
		return nil, err
	}

	t := block{
		Value: m.GetValue(),
	}

	return t, nil
}
