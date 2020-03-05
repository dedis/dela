package consensus

import (
	proto "github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
)

// Seal is the interface returned by a successful consensus.
type Seal interface {
	encoding.Packable

	GetFrom() blockchain.BlockID
	GetTo() blockchain.BlockID
	Verify(v crypto.Verifier, pubkeys []crypto.PublicKey) error
}

// SealFactory is an interface to create seals from messages.
type SealFactory interface {
	FromProto(msg proto.Message) (Seal, error)
}

// Consensus is an interface that provide primitives to propose a message to
// multiple participants so that it delivers a seal if enough of them agree.
type Consensus interface {
	GetSealFactory() SealFactory

	Propose(roster blockchain.Roster, from, proposal blockchain.Block) (Seal, error)
}
