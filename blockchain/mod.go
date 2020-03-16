package blockchain

import (
	"context"

	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	mino "go.dedis.ch/fabric/mino"
)

// Block is the interface of the unit of storage in the blockchain
type Block interface {
	encoding.Packable

	GetHash() []byte
}

// VerifiableBlock is an extension of a block so that its integrity can be
// verified from the genesis block.
type VerifiableBlock interface {
	Block

	Verify(crypto.Verifier) error
}

// BlockFactory provides primitives to create blocks from a untrusted source.
type BlockFactory interface {
	FromVerifiable(src proto.Message) (Block, error)
}

// Validator is the interface to implement to validate the generic payload
// stored in the block.
type Validator interface {
	Validate(data proto.Message) error
	Commit(data proto.Message) error
}

// Actor is a primitive created by the blockchain to propose new blocks.
type Actor interface {
	// InitChain creates a new chain with data as the payload and the players as
	// the first roster.
	InitChain(data proto.Message, players mino.Players) error

	// Store stores any representation of a data structure into a new block.
	// The implementation is responsible for any validations required.
	Store(data proto.Message, players mino.Players) error
}

// Blockchain is the interface that provides the primitives to interact with the
// blockchain.
type Blockchain interface {
	// GetBlockFactory returns the block factory.
	GetBlockFactory() (BlockFactory, error)

	// Listen starts to listen for messages and returns the actor that the
	// client can use to propose new blocks.
	Listen(validator Validator) (Actor, error)

	// GetBlock returns the latest block.
	GetBlock() (Block, error)

	// GetVerifiableBlock returns the latest block alongside with a proof from
	// the genesis block.
	GetVerifiableBlock() (VerifiableBlock, error)

	// Watch takes an observer that will be notified for each new block
	// definitely appended to the chain.
	Watch(ctx context.Context, obs Observer)
}
