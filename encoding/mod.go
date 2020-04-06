package encoding

import (
	"encoding"
	"io"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"golang.org/x/xerrors"
)

// Packable is an interface that provides primitives to pack data model into
// network messages.
type Packable interface {
	Pack() (proto.Message, error)
}

// BinaryMarshaler is an alias of the standard encoding library.
type BinaryMarshaler interface {
	encoding.BinaryMarshaler
}

// JSONMarshaler provides the primitives to encode a protobuf message into a
// JSON formatted text.
type JSONMarshaler interface {
	Marshal(io.Writer, proto.Message) error
}

// ProtoMarshaler is an interface to encode or decode Any messages.
type ProtoMarshaler interface {
	Marshal(pb proto.Message) ([]byte, error)
	MarshalAny(pb proto.Message) (*any.Any, error)
	UnmarshalAny(any *any.Any, pb proto.Message) error
	UnmarshalDynamicAny(any *any.Any) (proto.Message, error)
}

// ProtoEncoder is a default implementation of protobug encoding/decoding.
type ProtoEncoder struct{}

// NewProtoEncoder returns a new instance of the default protobuf encoder.
func NewProtoEncoder() ProtoEncoder {
	return ProtoEncoder{}
}

// Marshal encodes a protobuf message into bytes.
func (e ProtoEncoder) Marshal(pb proto.Message) ([]byte, error) {
	return proto.Marshal(pb)
}

// MarshalAny encodes a protobuf messages into the Any type.
func (e ProtoEncoder) MarshalAny(pb proto.Message) (*any.Any, error) {
	return ptypes.MarshalAny(pb)
}

// UnmarshalAny decodes a protobuf message from an Any type.
func (e ProtoEncoder) UnmarshalAny(any *any.Any, pb proto.Message) error {
	return ptypes.UnmarshalAny(any, pb)
}

// UnmarshalDynamicAny decodes an Any message dynamically.
func (e ProtoEncoder) UnmarshalDynamicAny(any *any.Any) (proto.Message, error) {
	if any == nil {
		return nil, xerrors.New("message is nil")
	}

	var da ptypes.DynamicAny
	err := ptypes.UnmarshalAny(any, &da)
	if err != nil {
		return nil, xerrors.Errorf("couldn't unmarshal dynamically: %v", err)
	}

	return da.Message, nil
}
