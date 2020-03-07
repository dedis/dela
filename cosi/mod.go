package cosi

import (
	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/mino"
)

// Cosigner is the interface that represents a participant.
type Cosigner interface {
	GetAddress() mino.Address
	GetPublicKey() crypto.PublicKey
}

// Hashable is the interface to implement to validate an incoming message
// for a collective signing. It will return the hash that will be signed.
type Hashable interface {
	Hash(in proto.Message) ([]byte, error)
}

// CollectiveSigning is the interface that provides the primitives to sign
// a message by members of a network.
type CollectiveSigning interface {
	GetPublicKey() crypto.PublicKey
	GetVerifier() crypto.Verifier
	Listen(Hashable) error
	Sign(msg proto.Message, signers ...Cosigner) (crypto.Signature, error)
}
