package cosi

import (
	"context"

	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
)

// CollectiveAuthority (or Cothority in short) is a set of participant to a
// collective signature.
type CollectiveAuthority interface {
	mino.Players
	PublicKeyIterator() crypto.PublicKeyIterator
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

// Actor is the listener of a collective signing instance. It provides a
// primitive to sign a message.
type Actor interface {
	Sign(ctx context.Context, msg Message, ca CollectiveAuthority) (crypto.Signature, error)
}

// CollectiveSigning is the interface that provides the primitives to sign a
// message by members of a network.
type CollectiveSigning interface {
	GetPublicKeyFactory() crypto.PublicKeyFactory
	GetSignatureFactory() crypto.SignatureFactory
	GetVerifier(ca CollectiveAuthority) (crypto.Verifier, error)
	Listen(Hashable) (Actor, error)
}
