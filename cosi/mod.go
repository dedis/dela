// Package cosi defines a collective signing protocol abstraction. A set of
// participants will work with each others to sign a unique message collectively
// in the sense that the protocol produces a single signature that will verify
// the full, or partial, aggregated public key.
//
// Related Papers:
//
// Enhancing Bitcoin Security and Performance with Strong Consistency via
// Collective Signing (2016)
// https://www.usenix.org/system/files/conference/usenixsecurity16/sec16_paper_kokoris-kogias.pdf
//
// On the Security of Two-Round Multi-Signatures (2019)
// https://eprint.iacr.org/2018/417.pdf
//
// Documentation Last Review: 05.10.2020
//
package cosi

import (
	"context"

	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
)

// Reactor is a collective signature event handler. Every participant must react
// to an incoming signature request from the leader, and this abstraction
// provides the primitive that allows to do so.
type Reactor interface {
	serde.Factory

	// Invoke is provided with the message and the address of the sender and it
	// should return the unique hash for this message, or an error for malformed
	// messages.
	Invoke(addr mino.Address, in serde.Message) ([]byte, error)
}

// Actor provides a primitive to sign a message.
type Actor interface {
	// Sign collects the signature of the collective authority and creates an
	// aggregated signature.
	Sign(ctx context.Context, msg serde.Message,
		ca crypto.CollectiveAuthority) (crypto.Signature, error)
}

// Threshold is a function that returns the threshold to reach for a given n,
// which means it is always positive and below or equal to n.
type Threshold func(int) int

// CollectiveSigning is the interface that provides the primitives to sign a
// message by members of a network.
type CollectiveSigning interface {
	// GetSigner returns the individual signer assigned to the instance. One
	// should not use it to verify a collective signature but only for identity
	// verification.
	GetSigner() crypto.Signer

	// GetPublicKeyFactory returns the aggregate public key factory.
	GetPublicKeyFactory() crypto.PublicKeyFactory

	// GetSignatureFactory returns the aggregate signature factory.
	GetSignatureFactory() crypto.SignatureFactory

	// GetVerifierFactory returns a factory that can create a verifier to check
	// the validity of a signature.
	GetVerifierFactory() crypto.VerifierFactory

	// SetThreshold updates the threshold required by a collective signature.
	SetThreshold(Threshold)

	// Listen starts the collective signing so that it will answer to requests.
	Listen(Reactor) (Actor, error)
}
