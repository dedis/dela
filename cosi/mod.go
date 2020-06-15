package cosi

import (
	"context"

	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
)

// Reactor is an event handler that demultiplex the events.
type Reactor interface {
	serde.Factory

	// Invoke is provided with the message and the address of the sender and it
	// should return the unique hash for this message.
	Invoke(addr mino.Address, in serde.Message) ([]byte, error)
}

// Actor is the listener of a collective signing instance. It provides a
// primitive to sign a message.
type Actor interface {
	// Sign collects the signature of the collective authority and creates an
	// aggregated signature.
	Sign(ctx context.Context, msg serde.Message,
		ca crypto.CollectiveAuthority) (crypto.Signature, error)
}

// CollectiveSigning is the interface that provides the primitives to sign a
// message by members of a network.
type CollectiveSigning interface {
	// GetSigner returns the individual signer assigned to the instance. One
	// should not use it to verify a collective signature but only for identity
	// verification.
	GetSigner() crypto.Signer

	// GetPublicKeyFactory returns the public key factory.
	GetPublicKeyFactory() serde.Factory

	// GetSignatureFactory returns the signature factory.
	GetSignatureFactory() serde.Factory

	// GetVerifierFactory returns a factory that can create a verifier to check
	// the validity of a signature.
	GetVerifierFactory() crypto.VerifierFactory

	// Listen starts the collective signing so that it will answer to requests.
	Listen(Reactor) (Actor, error)
}
