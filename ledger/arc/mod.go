// Package arc defines the interfaces for the Access Rights Control.
package arc

import (
	"encoding"
	"strings"

	"go.dedis.ch/dela/serdeng"
)

// Identity is an abstraction to uniquely identify a signer.
type Identity interface {
	serdeng.Message
	encoding.TextMarshaler
}

// AccessControl is an abstraction to verify if an identity has access to a
// specific rule.
type AccessControl interface {
	serdeng.Message

	Match(rule string, idents ...Identity) error
}

// Compile returns a compacted rule from the string segments.
func Compile(segments ...string) string {
	return strings.Join(segments, ":")
}
