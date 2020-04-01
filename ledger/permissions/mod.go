package permissions

import (
	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/encoding"
)

// Identity is an abstraction to uniquely identify a signer.
type Identity interface {
	encoding.Packable
}

// AccessControl is an abstraction to verify if an identity has access to a
// specific rule.
type AccessControl interface {
	Match(identity Identity, rule string) bool
}

type AccessControlFactory interface {
	FromProto(proto.Message) (AccessControl, error)
}
