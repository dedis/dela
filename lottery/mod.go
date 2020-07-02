package lottery

import (
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/ledger/arc"
	"go.dedis.ch/kyber/v3"
)

// Secret defines the primitives to run a Calypso-like app. In the case of
// Calypso it is mainly a wrapper arround DKG that provides a storage and
// authorization layer.
type Secret interface {
	// Listen should be called by each node to participate in the lottery
	Listen() error

	// Setup must be called only ONCE by one of the node to setup the lottery
	Setup(ca crypto.CollectiveAuthority, threshold int) (pubKey kyber.Point, err error)

	// GetPublicKey returns the collective public key. Returns an error if the
	// setup has not been done.
	GetPublicKey() (kyber.Point, error)

	Write(message EncryptedMessage, ac arc.AccessControl) (ID []byte, err error)
	Read(ID []byte, idents ...arc.Identity) (msg []byte, err error)
	UpdateAccess(ID []byte, ident arc.Identity, ac arc.AccessControl) error
}

// EncryptedMessage wraps the K, C arguments needed to decrypt a message. K is
// the ephemeral DH public key and C the blinded secret.
type EncryptedMessage interface {
	GetK() kyber.Point
	GetC() kyber.Point
}
