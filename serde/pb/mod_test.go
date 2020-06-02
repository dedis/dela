package pb

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func TestFactoryInput_Feed(t *testing.T) {
	input := factoryInput{
		data: []byte{},
	}

	ret := &empty.Empty{}
	err := input.Feed(ret)
	require.NoError(t, err)

	err = input.Feed(nil)
	require.EqualError(t, err, "invalid message type '<nil>'")

	input = factoryInput{data: []byte{0x1}}
	err = input.Feed(&empty.Empty{})
	require.EqualError(t, err,
		"couldn't unmarshal: proto: empty.Empty: illegal tag 0 (wire type 1)")
}

func TestSerializer_Serialize(t *testing.T) {
	s := NewSerializer()

	b := block{Value: "Hello World!"}

	buffer, err := s.Serialize(b)
	require.NoError(t, err)

	ret := &wrappers.StringValue{}
	err = proto.Unmarshal(buffer, ret)
	require.NoError(t, err)
	require.Equal(t, &wrappers.StringValue{Value: b.Value}, ret)

	_, err = s.Serialize(badMessage{err: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't visit message: oops")

	_, err = s.Serialize(badMessage{})
	require.EqualError(t, err, "visit should return a proto message")

	_, err = s.Serialize(badMessage{msg: (*empty.Empty)(nil)})
	require.EqualError(t, err,
		"couldn't marshal: proto: Marshal called with nil")
}

func TestSerializer_Deserialize(t *testing.T) {
	s := NewSerializer()

	b := block{Value: "Hello World!"}

	buffer, err := proto.Marshal(&wrappers.StringValue{Value: b.Value})
	require.NoError(t, err)

	m, err := s.Deserialize(buffer, blockFactory{})
	require.NoError(t, err)
	require.Equal(t, b, m)

	_, err = s.Deserialize(buffer, badFactory{})
	require.EqualError(t, err, "couldn't visit factory: oops")
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

type badMessage struct {
	serde.UnimplementedMessage

	msg interface{}
	err error
}

func (m badMessage) VisitProto() (interface{}, error) {
	return m.msg, m.err
}

type badFactory struct {
	serde.UnimplementedFactory
}

func (m badFactory) VisitProto(serde.FactoryInput) (serde.Message, error) {
	return nil, xerrors.New("oops")
}
