package consensus

import (
	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
)

// Proposal is the interface that the proposed data must implement to be
// accepted by the consensus.
type Proposal interface {
	encoding.Packable

	// GetIndex returns the index of the proposal from the first one.
	GetIndex() uint64

	// GetHash returns the hash of the proposal.
	GetHash() []byte

	// GetPreviousHash returns the hash of the previous proposal.
	GetPreviousHash() []byte
}

// Validator is the interface to implement to start a consensus.
type Validator interface {
	// Validate should return the proposal decoded from the message or
	// an error if it is invalid. It should also return the previous
	// proposal.
	Validate(addr mino.Address, message proto.Message) (curr Proposal, err error)

	// Commit should commit the proposal with the given identifier. The
	// implementation makes sure that the commit is atomic with the validation
	// so that no further locking is necessary.
	Commit(id []byte) error
}

// Chain is a verifiable lock between proposals.
type Chain interface {
	encoding.Packable

	// GetLastHash returns the last proposal hash of the chain.
	GetLastHash() []byte
}

// ChainFactory is a factory to decodes chain from protobuf messages.
type ChainFactory interface {
	// FromProto returns the instance of the chain decoded from the message.
	FromProto(pb proto.Message) (Chain, error)
}

// Actor is the primitive to send proposals to a consensus implementation.
type Actor interface {
	// Propose performs the consensus algorithm. The list of participants is
	// left to the implementation.
	Propose(proposal Proposal) error

	// Close must clean the resources of the actor.
	Close() error
}

// Consensus is an interface that provides primitives to propose data to a set
// of participants. They will validate the proposal according to the validator.
type Consensus interface {
	// GetChainFactory returns the chain factory.
	GetChainFactory() (ChainFactory, error)

	// GetChain returns a valid chain to the given identifier.
	GetChain(to []byte) (Chain, error)

	// Listen starts to listen for consensus messages.
	Listen(h Validator) (Actor, error)
}
