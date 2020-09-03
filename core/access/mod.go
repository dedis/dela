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

// Access is an abstraction to verify if an identity has access to a
// specific rule.
type Access interface {
	serde.Message

	Match(rule string, idents ...Identity) error
}

// Factory is the factory interface to deserialize accesses.
type Factory interface {
	serde.Factory

	AccessOf(serde.Context, []byte) (Access, error)
}

// Compile returns a compacted rule from the string segments.
func Compile(segments ...string) string {
	return strings.Join(segments, ":")
}
