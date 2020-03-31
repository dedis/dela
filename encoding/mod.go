package encoding

import (
	"encoding"
	"io"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"golang.org/x/xerrors"
)

// Packable is an interface that provides primitives to pack data model into
// network messages.
type Packable interface {
	Pack(encoder ProtoMarshaler) (proto.Message, error)
}

// BinaryMarshaler is an alias of the standard encoding library.
type BinaryMarshaler interface {
	encoding.BinaryMarshaler
}

// ProtoMarshaler is an interface to encode or decode Any messages.
type ProtoMarshaler interface {
	// Pack will pack the object into a protobuf message.
	Pack(p Packable) (proto.Message, error)

	// PackAny will pack the object into an any message.
	PackAny(p Packable) (*any.Any, error)

	// Marshal is a deterministic marshaling of the message into the writer.
	Marshal(io.Writer, proto.Message) error

	// MarshalAny encodes a protobuf message into an any message.
	MarshalAny(pb proto.Message) (*any.Any, error)

	// UnmarshalAny decodes back the any message into the protobuf message.
	UnmarshalAny(any *any.Any, pb proto.Message) error

	// UnmarshalDynamicAny decodes the any message into a new instance of the
	// protobuf message.
	UnmarshalDynamicAny(any *any.Any) (proto.Message, error)
}

// ProtoEncoder is a default implementation of protobug encoding/decoding.
type ProtoEncoder struct {
	marshaler *jsonpb.Marshaler
}

// NewProtoEncoder returns a new instance of the default protobuf encoder.
func NewProtoEncoder() ProtoEncoder {
	return ProtoEncoder{
		marshaler: &jsonpb.Marshaler{},
	}
}

func (e ProtoEncoder) Pack(p Packable) (proto.Message, error) {
	pb, err := p.Pack(e)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack '%T': %v", p, err)
	}

	return pb, nil
}

func (e ProtoEncoder) PackAny(p Packable) (*any.Any, error) {
	pb, err := p.Pack(e)
	if err != nil {
		return nil, err
	}

	pbAny, err := ptypes.MarshalAny(pb)
	if err != nil {
		return nil, err
	}

	return pbAny, nil
}

func (e ProtoEncoder) Marshal(w io.Writer, pb proto.Message) error {
	return e.marshaler.Marshal(w, pb)
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
