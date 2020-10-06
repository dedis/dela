// Package crypto defines cryptographic primitives shared by the different
// modules of Dela.
//
// It defines the abstraction of a public and a private key and their
// combination where a private key can create a signature for a given message,
// while the public key can verify the association of those two elements.
//
// A signer is a unique entity that possesses both a public and an associated
// private key. It is assumed that the combination is a unique identity. This
// abstraction hides the logic of a private key.
//
// For the aggregation of those primitives, a verifier abstraction is defined to
// provide the primitives to verify using the aggregate instead of a single one.
//
// Documentation Last Review: 05.10.2020
//
package crypto

import (
	"encoding"
	"hash"

	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
)

// HashFactory is an interface to produce a hash digest.
type HashFactory interface {
	New() hash.Hash
}

// RandGenerator is an interface to generate random values with a fully seeded
// random source.
type RandGenerator interface {
	// Read populates the buffer with random bytes up to a size that is
	// returned alongside an error if something goes wrong.
	Read([]byte) (int, error)
}

// PublicKey is a public identity that can be used to verify a signature.
type PublicKey interface {
	encoding.BinaryMarshaler
	encoding.TextMarshaler
	serde.Message

	// Verify returns nil if the signature matches the message, otherwise an
	// error is returned.
	Verify(msg []byte, signature Signature) error

	// Equal returns true when both objects are similar.
	Equal(other interface{}) bool
}

// PublicKeyFactory is a factory to decode public keys.
type PublicKeyFactory interface {
	serde.Factory

	// PublicKeyOf populates the public key associated to the data if
	// appropriate, otherwise it returns an error.
	PublicKeyOf(ctx serde.Context, data []byte) (PublicKey, error)

	// FromBytes returns the public key associated to the data if appropriate,
	// otherwise it returns an error.
	FromBytes(data []byte) (PublicKey, error)
}

// PublicKeyIterator is an iterator over the list of public keys of a
// collective authority.
type PublicKeyIterator interface {
	// Seek moves the iterator to a specific index.
	Seek(int)

	// HasNext returns true if a public key is available, false if the iterator
	// is exhausted.
	HasNext() bool

	// GetNext returns the next public key in case HasNext returns true,
	// otherwise no assumption can be done.
	GetNext() PublicKey
}

// Signature is a verifiable element for a unique message.
type Signature interface {
	encoding.BinaryMarshaler
	serde.Message

	// Equal returns true if both objects are similar.
	Equal(other Signature) bool
}

// SignatureFactory is a factory to decode signatures.
type SignatureFactory interface {
	serde.Factory

	// SignatureOf returns a signature associated with the data if appropriate,
	// otherwise it returns an error.
	SignatureOf(ctx serde.Context, data []byte) (Signature, error)
}

// Verifier provides the primitive to verify a signature w.r.t. a message.
type Verifier interface {
	// Verify returns nil if the signature matches the message for the public
	// key internal to the verifier.
	Verify(msg []byte, signature Signature) error
}

// VerifierFactory provides the primitives to create a verifier.
type VerifierFactory interface {
	// FromAuthority returns a verifier that will use the authority to generate
	// a public key.
	FromAuthority(ca CollectiveAuthority) (Verifier, error)

	// FromArray returns a verifier that will use the list of public keys to
	// generate one.
	FromArray(keys []PublicKey) (Verifier, error)
}

// Signer provides the primitives to sign and verify signatures.
type Signer interface {
	// GetPublicKeyFactory returns a factory that can deserialize public keys of
	// the same type as the signer.
	GetPublicKeyFactory() PublicKeyFactory

	// GetSignatureFactory returns a factory that can deserialize signatures of
	// the same type as the signer.
	GetSignatureFactory() SignatureFactory

	// GetPublicKey returns the public key of the signer.
	GetPublicKey() PublicKey

	// Sign returns a signature that will match the message for the signer
	// public key.
	Sign(msg []byte) (Signature, error)
}

// AggregateSigner offers the same primitives as the Signer interface but
// also includes a primitive to aggregate signatures into one.
type AggregateSigner interface {
	Signer

	// GetVerifierFactory returns the factory that can deserialize verifiers of
	// the same type as the signer.
	GetVerifierFactory() VerifierFactory

	// Aggregate returns the aggregate signature of the ones in parameter.
	Aggregate(signatures ...Signature) (Signature, error)
}

// CollectiveAuthority is a set of participants with each of them being
// associated to a Mino address and a public key.
type CollectiveAuthority interface {
	mino.Players

	// GetPublicKey returns the public key and its index of the corresponding
	// address if any matches. An index < 0 means no correspondance found.
	GetPublicKey(addr mino.Address) (PublicKey, int)

	// PublicKeyIterator creates a public key iterator that iterates over the
	// list of public keys and is consistent with the address iterator.
	PublicKeyIterator() PublicKeyIterator
}
