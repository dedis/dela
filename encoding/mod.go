package encoding

import (
	"encoding"

	"github.com/golang/protobuf/proto"
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

// AnyMarshaler is an interface to encode or decode Any messages.
type AnyMarshaler interface {
	MarshalAny(pb proto.Message) (*any.Any, error)
	UnmarshalAny(any *any.Any, pb proto.Message) error
}
