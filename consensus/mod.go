package consensus

import (
	"github.com/golang/protobuf/proto"
	"go.dedis.ch/dela/encoding"
	"go.dedis.ch/dela/mino"
)

type Reactor interface {
	InvokeValidate(mino.Address, proto.Message) ([]byte, error)

	InvokeCommit(id []byte) error
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
	Propose(proto.Message) error

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
	Listen(Reactor) (Actor, error)

	// Store updates the local chain and return an error if they don't match.
	Store(Chain) error
}
