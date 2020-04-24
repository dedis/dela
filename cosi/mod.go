package cosi

import (
	"context"

	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
)

// Hashable is the interface to implement to validate an incoming message for a
// collective signing. It will return the hash that will be signed.
type Hashable interface {
	// Hash is provided with the message and the address of the sender and it
	// should return the unique hash for this message.
	Hash(addr mino.Address, in proto.Message) ([]byte, error)
}

// Message is the type of input that can be provided to a collective signing
// protocol.
type Message interface {
	encoding.Packable

	// GetHash returns the unique hash of the message.
	GetHash() []byte
}

// Actor is the listener of a collective signing instance. It provides a
// primitive to sign a message.
type Actor interface {
	// Sign collects the signature of the collective authority and creates an
	// aggregated signature.
	Sign(ctx context.Context, msg Message, ca crypto.CollectiveAuthority) (crypto.Signature, error)
}

// CollectiveSigning is the interface that provides the primitives to sign a
// message by members of a network.
type CollectiveSigning interface {
	// GetPublicKeyFactory returns the public key factory.
	GetPublicKeyFactory() crypto.PublicKeyFactory

	// GetSignatureFactory returns the signature factory.
	GetSignatureFactory() crypto.SignatureFactory

	// GetVerifier returns a verifier that can verify the signature created from
	// a collective signing.
	GetVerifierFactory() crypto.VerifierFactory

	// Listen starts the collective signing so that it will answer to requests.
	Listen(Hashable) (Actor, error)
}
