package consensus

import (
	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
)

// Proposal is the interface that the proposed data must implement to be
// accepted by the consensus.
type Proposal interface {
	encoding.Packable

	GetHash() []byte
}

// Validator is the interface to implement to start a consensus.
type Validator interface {
	Validate(previous []byte, message proto.Message) (Proposal, error)
}

// Chain is a verifiable lock between proposals.
type Chain interface {
	encoding.Packable

	Verify(verifier crypto.Verifier, pubkeys []crypto.PublicKey) error
}

// ChainFactory is a factory to decodes chain from protobuf messages.
type ChainFactory interface {
	FromProto(pb proto.Message) (Chain, error)
}

type Participant interface {
	GetAddress() *mino.Address
	GetPublicKey() crypto.PublicKey
}

// Consensus is an interface that provides primitives to propose data to a set
// of participants. They will validate the proposal according to the validator.
type Consensus interface {
	GetChain(from uint64, to uint64) Chain

	Listen(h Validator) error

	Propose(proposal Proposal, participants ...Participant) error
}
