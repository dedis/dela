package cosi

import (
	"github.com/golang/protobuf/proto"
	"go.dedis.ch/m/blockchain"
	"go.dedis.ch/m/crypto"
)

// CollectiveSigning is the interface that provides the primitives to sign
// a message by members of a network.
type CollectiveSigning interface {
	PublicKey() crypto.PublicKey
	Sign(ro blockchain.Roster, msg proto.Message) (crypto.Signature, error)
	MakeVerifier() crypto.Verifier
}
