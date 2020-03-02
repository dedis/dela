package crypto

import (
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"go.dedis.ch/fabric/encoding"
)

// PublicKey is a public identity that can be used to verify a signature.
type PublicKey interface {
	encoding.Packable
}

// PublicKeyFactory is a factory to create public keys.
type PublicKeyFactory interface {
	FromAny(src *any.Any) (PublicKey, error)
	FromProto(src proto.Message) (PublicKey, error)
}

// Signature is a verifiable element for a unique message.
type Signature interface {
	encoding.Packable
}

// SignatureFactory is a factory to create BLS signature.
type SignatureFactory interface {
	FromAny(src *any.Any) (Signature, error)
	FromProto(src proto.Message) (Signature, error)
}

// Verifier provides the primitive to verify a signature w.r.t. a message.
type Verifier interface {
	GetPublicKeyFactory() PublicKeyFactory
	GetSignatureFactory() SignatureFactory

	Verify(publicKeys []PublicKey, msg []byte, signature Signature) error
}

// Signer provides the primitives to sign and verify signatures.
type Signer interface {
	Verifier

	PublicKey() PublicKey
	Sign(msg []byte) (Signature, error)
}

// AggregateSigner offers the same primitives as the Signer interface but
// also includes a primitive to aggregate signatures into one.
type AggregateSigner interface {
	Signer

	Aggregate(signatures ...Signature) (Signature, error)
}
