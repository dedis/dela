package blockchain

import (
	"context"

	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serdeng"
)

// Payload is the interface to implement to store data in a block.
type Payload interface {
	serdeng.Message
	serdeng.Fingerprinter
}

// Block is the interface of the unit of storage in the blockchain
type Block interface {
	serdeng.Message

	// GetIndex returns the index since the genesis block.
	GetIndex() uint64

	// GetHash returns the fingerprint of the block.
	GetHash() []byte

	// GetPayload returns the payload of the block.
	GetPayload() Payload
}

// VerifiableBlock is an extension of a block so that its integrity can be
// verified from the genesis block.
type VerifiableBlock interface {
	Block
}

// Reactor is the interface to implement to react to blockchain events.
type Reactor interface {
	serdeng.Factory

	InvokeValidate(data serdeng.Message) (Payload, error)

	InvokeCommit(payload Payload) error
}

// Actor is a primitive created by the blockchain to propose new blocks.
type Actor interface {
	// Setup initializes a new chain by creating the genesis block with the
	// given payload and players to be the roster of the genesis block.
	Setup(data Payload, players mino.Players) error

	// Store stores any representation of a data structure into a new block.
	// The implementation is responsible for any validations required.
	Store(data serdeng.Message, players mino.Players) error
}

// Blockchain is the interface that provides the primitives to interact with the
// blockchain.
type Blockchain interface {
	// Listen starts to listen for messages and returns the actor that the
	// client can use to propose new blocks.
	Listen(Reactor) (Actor, error)

	// GetBlock returns the latest block.
	GetBlock() (Block, error)

	// GetVerifiableBlock returns the latest block alongside with a proof from
	// the genesis block.
	GetVerifiableBlock() (VerifiableBlock, error)

	// Watch returns a channel that will be filled by new incoming blocks. The
	// caller is responsible for cancelling the context to clean the observer.
	Watch(ctx context.Context) <-chan Block
}
