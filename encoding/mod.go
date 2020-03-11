package encoding

import (
	"encoding"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
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

// ProtoMarshaler is an interface to encode or decode Any messages.
type ProtoMarshaler interface {
	Marshal(pb proto.Message) ([]byte, error)
	MarshalAny(pb proto.Message) (*any.Any, error)
	UnmarshalAny(any *any.Any, pb proto.Message) error
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
