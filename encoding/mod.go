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
	// MarshalStable is a deterministic marshaling of the message into the writer.
	MarshalStable(io.Writer, proto.Message) error

	// MarshalAny encodes a protobuf message into an any message.
	MarshalAny(pb proto.Message) (*any.Any, error)

	// Pack will pack the object into a protobuf message.
	Pack(p Packable) (proto.Message, error)

	// PackAny will pack the object into an any message.
	PackAny(p Packable) (*any.Any, error)

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
//
// - implements encoding.ProtoMarshaler
func NewProtoEncoder() ProtoEncoder {
	return ProtoEncoder{
		marshaler: &jsonpb.Marshaler{},
	}
}

// Pack implements encoding.ProtoMarshaler. It returns the protobuf message of
// the packable object.
func (e ProtoEncoder) Pack(p Packable) (proto.Message, error) {
	pb, err := p.Pack(e)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack '%T': %v", p, err)
	}

	return pb, nil
}

// PackAny implements encoding.ProtoMarshaler. It returns the protobuf message
// wrapped into an any object.
func (e ProtoEncoder) PackAny(p Packable) (*any.Any, error) {
	if p == nil {
		return nil, xerrors.New("message is nil")
	}

	pb, err := p.Pack(e)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack '%T': %v", p, err)
	}

	pbAny, err := ptypes.MarshalAny(pb)
	if err != nil {
		return nil, xerrors.Errorf("couldn't wrap '%T' to any: %v", pb, err)
	}

	return pbAny, nil
}

// MarshalStable implements encoding.ProtoMarshaler. It writes the message into
// the writer in a stable way so that it can be reproduced in a different
// environment. It is different from the usual protobuf marshaling as this last
// is not stable.
func (e ProtoEncoder) MarshalStable(w io.Writer, pb proto.Message) error {
	err := e.marshaler.Marshal(w, pb)
	if err != nil {
		return xerrors.Errorf("stable serialization failed: %v", err)
	}

	return nil
}

// MarshalAny implements encoding.ProtoMarshaler. It encodes a protobuf messages
// into the Any type.
func (e ProtoEncoder) MarshalAny(pb proto.Message) (*any.Any, error) {
	res, err := ptypes.MarshalAny(pb)
	if err != nil {
		return nil, xerrors.Errorf("couldn't wrap '%T' to any: %v", pb, err)
	}

	return res, nil
}

// UnmarshalAny implements encoding.ProtoMarshaler. It decodes a protobuf
// message from an Any type.
func (e ProtoEncoder) UnmarshalAny(any *any.Any, pb proto.Message) error {
	err := ptypes.UnmarshalAny(any, pb)
	if err != nil {
		return xerrors.Errorf("couldn't unwrap '%T' to '%T': %v", any, pb, err)
	}

	return nil
}

// UnmarshalDynamicAny implements encoding.ProtoMarshaler. It decodes an Any
// message dynamically.
func (e ProtoEncoder) UnmarshalDynamicAny(any *any.Any) (proto.Message, error) {
	if any == nil {
		return nil, xerrors.New("message is nil")
	}

	var da ptypes.DynamicAny
	err := ptypes.UnmarshalAny(any, &da)
	if err != nil {
		return nil, xerrors.Errorf("couldn't dynamically unwrap: %v", err)
	}

	return da.Message, nil
}
