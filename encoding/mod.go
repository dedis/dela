package encoding

import (
	"encoding"

	"github.com/golang/protobuf/proto"
)

// Packable is an interface that provides primitives to pack data model into
// network messages.
type Packable interface {
	encoding.BinaryMarshaler

	Pack() (proto.Message, error)
}
