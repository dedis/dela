package blockchain

import (
	"context"

	"github.com/golang/protobuf/proto"
	"go.dedis.ch/m/crypto"
)

//go:generate protoc -I ./ --proto_path=../ --go_out=Mmino/messages.proto=go.dedis.ch/m/mino:. ./messages.proto

// Roster is a set of identifiable addresses.
type Roster []*Conode

// BlockFactory provides primitives to create blocks from a untrusted source.
type BlockFactory interface {
	FromPrevious(previous interface{}, data proto.Message) (interface{}, error)

	FromVerifiable(src *VerifiableBlock, originPublicKeys []crypto.PublicKey) (interface{}, error)
}

// Blockchain is the interface that provides the primitives to interact with the
// blockchain.
type Blockchain interface {
	GetBlockFactory() BlockFactory

	// Store stores any representation of a data structure into a new block.
	// The implementation is responsible for any validations required.
	Store(roster Roster, data proto.Message) error

	// GetBlock returns the latest block.
	GetBlock() (*Block, error)

	// GetVerifiableBlock returns the latest block alongside with a proof from
	// the genesis block.
	GetVerifiableBlock() (*VerifiableBlock, error)

	// Watch takes an observer that will be notified for each new block
	// definitely appended to the chain.
	Watch(ctx context.Context, obs Observer)
}
