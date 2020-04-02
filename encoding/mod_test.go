package encoding

import (
	"bytes"
	"testing"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

func TestProtoEncoder_Pack(t *testing.T) {
	enc := NewProtoEncoder()

	pb, err := enc.Pack(fakePackable{})
	require.NoError(t, err)
	require.IsType(t, (*empty.Empty)(nil), pb)

	_, err = enc.Pack(nil)
	require.EqualError(t, err, "message is nil")

	_, err = enc.Pack(fakePackable{err: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't pack 'encoding.fakePackable': oops")
}

func TestProtoEncoder_PackAny(t *testing.T) {
	enc := NewProtoEncoder()

	fake, err := enc.PackAny(fakePackable{})
	require.NoError(t, err)
	require.IsType(t, (*any.Any)(nil), fake)

	_, err = enc.PackAny(nil)
	require.EqualError(t, err, "message is nil")

	_, err = enc.PackAny(fakePackable{err: xerrors.New("oops")})
	require.EqualError(t, err, "couldn't pack 'encoding.fakePackable': oops")

	_, err = enc.PackAny(emptyPackable{})
	require.EqualError(t, err,
		"couldn't wrap '<nil>' to any: proto: Marshal called with nil")
}

func TestProtoEncoder_MarshalStable(t *testing.T) {
	enc := NewProtoEncoder()
	buffer := new(bytes.Buffer)

	err := enc.MarshalStable(buffer, &wrappers.StringValue{Value: "abc"})
	require.NoError(t, err)
	// JSON format is exploited for stable serialization.
	require.Equal(t, "\"abc\"", buffer.String())

	err = enc.MarshalStable(nil, nil)
	require.EqualError(t, err, "message is nil")

	err = enc.MarshalStable(nil, &empty.Empty{})
	require.EqualError(t, err, "writer is nil")

	err = enc.MarshalStable(buffer, &badMessage{})
	require.EqualError(t, err, "stable serialization failed: oops")
}

func TestProtoEncoder_MarshalAny(t *testing.T) {
	enc := NewProtoEncoder()

	res, err := enc.MarshalAny(&empty.Empty{})
	require.NoError(t, err)
	require.IsType(t, (*any.Any)(nil), res)

	_, err = enc.MarshalAny(nil)
	require.EqualError(t, err, "message is nil")

	_, err = enc.MarshalAny(&badMessage{})
	require.EqualError(t, err,
		"couldn't wrap '*encoding.badMessage' to any: oops")
}

func TestProtoEncoder_UnmarshalAny(t *testing.T) {
	pb, err := ptypes.MarshalAny(&wrappers.UInt64Value{Value: 123})
	require.NoError(t, err)

	enc := NewProtoEncoder()
	msg := &wrappers.UInt64Value{}
	err = enc.UnmarshalAny(pb, msg)
	require.NoError(t, err)
	require.Equal(t, uint64(123), msg.GetValue())

	err = enc.UnmarshalAny(nil, nil)
	require.EqualError(t, err, "message is nil")

	err = enc.UnmarshalAny(&any.Any{}, nil)
	require.EqualError(t, err, "receiver is nil")

	err = enc.UnmarshalAny(&any.Any{}, msg)
	require.EqualError(t, err,
		"couldn't unwrap '*any.Any' to '*wrappers.UInt64Value': message type url \"\" is invalid")
}

func TestProtoEncoder_UnmarshalDynamicAny(t *testing.T) {
	packed, err := ptypes.MarshalAny(&wrappers.UInt64Value{Value: 123})
	require.NoError(t, err)

	enc := NewProtoEncoder()
	msg, err := enc.UnmarshalDynamicAny(packed)
	require.NoError(t, err)
	require.IsType(t, (*wrappers.UInt64Value)(nil), msg)

	_, err = enc.UnmarshalDynamicAny(nil)
	require.EqualError(t, err, "message is nil")

	_, err = enc.UnmarshalDynamicAny(&any.Any{})
	require.Error(t, err)
	require.EqualError(t, err,
		"couldn't dynamically unwrap: message type url \"\" is invalid")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakePackable struct {
	err error
}

func (p fakePackable) Pack(ProtoMarshaler) (proto.Message, error) {
	return &empty.Empty{}, p.err
}

type emptyPackable struct{}

func (p emptyPackable) Pack(ProtoMarshaler) (proto.Message, error) {
	return nil, nil
}

type badMessage struct {
	proto.Message
}

func (m *badMessage) Marshal() ([]byte, error) {
	return nil, xerrors.New("oops")
}

func (m *badMessage) MarshalJSONPB(*jsonpb.Marshaler) ([]byte, error) {
	return nil, xerrors.New("oops")
}
