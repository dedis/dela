// Package access defines the interfaces for the Access Rights Control.
package access

import (
	"encoding"

	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/serde"
)

// Identity is an abstraction to uniquely identify a signer.
type Identity interface {
	serde.Message

	encoding.TextMarshaler

	Equal(other interface{}) bool
}

type Credentials interface {
	GetID() []byte
	GetRule() string
}

type Service interface {
	Match(store store.Readable, creds Credentials, idents ...Identity) error

	Grant(store store.Snapshot, creds Credentials, idents ...Identity) error
}
