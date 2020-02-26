package encoding

import "github.com/golang/protobuf/proto"

// Packable is an interface that provides primitives to pack data model into
// network messages.
type Packable interface {
	Pack() (proto.Message, error)
}
