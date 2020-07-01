package lottery

import (
	"go.dedis.ch/dela/ledger/arc"
	"go.dedis.ch/kyber/v3"
)

// Secret defines the primitives to run a Calypso-like app. In the case of
// Calypso it is mainly a wrapper arround DKG that provides a storage and
// authorization layer.
type Secret interface {
	Setup() (pubKey kyber.Point, err error)
	Write(message EncryptedMessage, ac arc.AccessControl) (ID []byte, err error)
	Read(ID []byte, idents ...arc.Identity) (msg []byte, err error)
	UpdateAccess(ID []byte, ident arc.Identity, ac arc.AccessControl) error
}

// Policy defines an interface to check the authorization of an action
type Policy interface {
	Match() error
}

// Approval defines an interface to describe a granted authorization
type Approval interface {
	GetIdentities() [][]byte
}

// EncryptedMessage wraps the K, C arguments needed to decrypt a message. K is
// the ephemeral DH public key and C the blinded secret.
type EncryptedMessage interface {
	GetK() kyber.Point
	GetC() kyber.Point
}
