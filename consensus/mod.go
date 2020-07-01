package consensus

import (
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
)

type Reactor interface {
	serde.Factory

	InvokeGenesis() ([]byte, error)

	InvokeValidate(mino.Address, serde.Message) ([]byte, error)

	InvokeCommit(id []byte) error
}

// Chain is a verifiable lock between proposals.
type Chain interface {
	serde.Message

	GetTo() []byte
}

type ChainFactory interface {
	serde.Factory

	ChainOf(serde.Context, []byte) (Chain, error)
}

// Actor is the primitive to send proposals to a consensus implementation.
type Actor interface {
	// Propose performs the consensus algorithm. The list of participants is
	// left to the implementation.
	Propose(serde.Message) error

	// Close must clean the resources of the actor.
	Close() error
}

// Consensus is an interface that provides primitives to propose data to a set
// of participants. They will validate the proposal according to the validator.
type Consensus interface {
	GetChainFactory() ChainFactory

	// GetChain returns a valid chain to the given identifier.
	GetChain(to []byte) (Chain, error)

	// Listen starts to listen for consensus messages.
	Listen(Reactor) (Actor, error)
}
