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

	// GetHash returns the hash of the proposal.
	GetHash() []byte

	// GetPreviousHash returns the hash of the previous proposal.
	GetPreviousHash() []byte

	// GetVerifier returns a verifier that can be used to assert the validity of
	// the signatures from the participants of this proposal.
	GetVerifier() crypto.Verifier
}

// Validator is the interface to implement to start a consensus.
type Validator interface {
	// Validate should return the proposal decoded from the message or
	// an error if it is invalid. It should also return the previous
	// proposal.
	Validate(message proto.Message) (curr Proposal, err error)

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

	// Verify returns nil if the integriy of the chain is valid, otherwise
	// an error.
	Verify(verifier crypto.Verifier) error
}

// ChainFactory is a factory to decodes chain from protobuf messages.
type ChainFactory interface {
	// FromProto returns the instance of the chain decoded from the message.
	FromProto(pb proto.Message) (Chain, error)
}

// Actor is the primitive to send proposals to a consensus implementation.
type Actor interface {
	// Propose performs the consensus algorithm using the list of nodes
	// as participants.
	Propose(proposal Proposal, players mino.Players) error
}

// Consensus is an interface that provides primitives to propose data to a set
// of participants. They will validate the proposal according to the validator.
type Consensus interface {
	// GetChainFactory returns the chain factory.
	GetChainFactory() ChainFactory

	// GetChain returns a valid chain to the given identifier.
	GetChain(id []byte) (Chain, error)

	// Listen starts to listen for consensus messages.
	Listen(h Validator) (Actor, error)
}
