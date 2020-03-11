package cosi

import (
	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
)

// PublicKeyIterator is an iterator over the list of public keys of a
// collective authority.
type PublicKeyIterator interface {
	Next() bool
	Get() crypto.PublicKey
}

// CollectiveAuthority (or Cothority in short) is a set of participant to a
// collective signature.
type CollectiveAuthority interface {
	mino.Membership
	PublicKeyIterator() PublicKeyIterator
}

// Hashable is the interface to implement to validate an incoming message for a
// collective signing. It will return the hash that will be signed.
type Hashable interface {
	Hash(in proto.Message) ([]byte, error)
}

// Message is the type of input that can be provided to a collective signing
// protocol.
type Message interface {
	encoding.Packable

	GetHash() []byte
}

// CollectiveSigning is the interface that provides the primitives to sign a
// message by members of a network.
type CollectiveSigning interface {
	GetPublicKeyFactory() crypto.PublicKeyFactory
	GetSignatureFactory() crypto.SignatureFactory
	GetVerifier(ca CollectiveAuthority) crypto.Verifier
	Listen(Hashable) error
	Sign(msg Message, ca CollectiveAuthority) (crypto.Signature, error)
}
