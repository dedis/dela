// Package access defines the interfaces for the Access Rights Control.
package access

import (
	"encoding"
	"strings"

	"go.dedis.ch/dela/serde"
)

// Identity is an abstraction to uniquely identify a signer.
type Identity interface {
	serde.Message
	encoding.TextMarshaler
}

// AccessControl is an abstraction to verify if an identity has access to a
// specific rule.
type AccessControl interface {
	serde.Message

	Match(rule string, idents ...Identity) error
}

type AccessControlFactory interface {
	serde.Factory

	AccessOf(serde.Context, []byte) (AccessControl, error)
}

// Compile returns a compacted rule from the string segments.
func Compile(segments ...string) string {
	return strings.Join(segments, ":")
}
