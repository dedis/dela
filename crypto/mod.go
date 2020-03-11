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

// PublicKey is a public identity that can be used to verify a signature.
type PublicKey interface {
	encoding.Packable
	encoding.BinaryMarshaler
}

// PublicKeyFactory is a factory to create public keys.
type PublicKeyFactory interface {
	FromProto(src proto.Message) (PublicKey, error)
}

// Signature is a verifiable element for a unique message.
type Signature interface {
	encoding.Packable
	encoding.BinaryMarshaler
}

// SignatureFactory is a factory to create BLS signature.
type SignatureFactory interface {
	FromProto(src proto.Message) (Signature, error)
}

// Verifier provides the primitive to verify a signature w.r.t. a message.
type Verifier interface {
	GetPublicKeyFactory() PublicKeyFactory

	Verify(msg []byte, signature Signature) error
}

type VerifierFactory interface {
	Create(publicKeys []PublicKey) Verifier
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
