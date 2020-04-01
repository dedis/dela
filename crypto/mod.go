package crypto

import (
	"hash"

	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/encoding"
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
	encoding.Packable
	encoding.BinaryMarshaler

	Verify(msg []byte, signature Signature) error

	Equal(other PublicKey) bool
}

// PublicKeyFactory is a factory to create public keys.
type PublicKeyFactory interface {
	FromProto(src proto.Message) (PublicKey, error)
}

// PublicKeyIterator is an iterator over the list of public keys of a
// collective authority.
type PublicKeyIterator interface {
	HasNext() bool
	GetNext() PublicKey
}

// Signature is a verifiable element for a unique message.
type Signature interface {
	encoding.Packable
	encoding.BinaryMarshaler

	Equal(other Signature) bool
}

// SignatureFactory is a factory to create BLS signature.
type SignatureFactory interface {
	FromProto(src proto.Message) (Signature, error)
}

// Verifier provides the primitive to verify a signature w.r.t. a message.
type Verifier interface {
	Verify(msg []byte, signature Signature) error
}

// VerifierFactory provides the primitives to create a verifier.
type VerifierFactory interface {
	FromIterator(iter PublicKeyIterator) (Verifier, error)
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
