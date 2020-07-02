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
	Read([]byte) (int, error)
}

// PublicKey is a public identity that can be used to verify a signature.
type PublicKey interface {
	encoding.BinaryMarshaler
	encoding.TextMarshaler
	serde.Message

	Verify(msg []byte, signature Signature) error

	Equal(other PublicKey) bool
}

// PublicKeyFactory is a factory to decode public keys.
type PublicKeyFactory interface {
	serde.Factory

	PublicKeyOf(serde.Context, []byte) (PublicKey, error)
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

	Equal(other Signature) bool
}

// SignatureFactory is a factory to decode signatures.
type SignatureFactory interface {
	serde.Factory

	SignatureOf(serde.Context, []byte) (Signature, error)
}

// Verifier provides the primitive to verify a signature w.r.t. a message.
type Verifier interface {
	Verify(msg []byte, signature Signature) error
}

// VerifierFactory provides the primitives to create a verifier.
type VerifierFactory interface {
	FromAuthority(ca CollectiveAuthority) (Verifier, error)
	FromArray(keys []PublicKey) (Verifier, error)
}

// Signer provides the primitives to sign and verify signatures.
type Signer interface {
	GetVerifierFactory() VerifierFactory
	GetPublicKeyFactory() PublicKeyFactory
	GetSignatureFactory() SignatureFactory
	GetPublicKey() PublicKey
	Sign(msg []byte) (Signature, error)
}

// AggregateSigner offers the same primitives as the Signer interface but
// also includes a primitive to aggregate signatures into one.
type AggregateSigner interface {
	Signer

	Aggregate(signatures ...Signature) (Signature, error)
}

// CollectiveAuthority (or Cothority in short) is a set of participant to a
// collective signature.
type CollectiveAuthority interface {
	mino.Players

	// GetPublicKey returns the public key and its index of the corresponding
	// address if any matches. An index < 0 means no correspondance found.
	GetPublicKey(addr mino.Address) (PublicKey, int)

	// PublicKeyIterator creates an public key iterator that iterates over the
	// list of public keys.
	PublicKeyIterator() PublicKeyIterator
}
