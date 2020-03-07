package blockchain

import (
	"bytes"
	"context"
	fmt "fmt"

	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	mino "go.dedis.ch/fabric/mino"
)

// BlockID is a unique identifier for each block which is composed
// of its hash.
type BlockID [32]byte

// NewBlockID returns an instance of a block identifier.
func NewBlockID(buffer []byte) BlockID {
	id := BlockID{}
	copy(id[:], buffer)
	return id
}

// Bytes returns the slice of bytes of the identifier.
func (id BlockID) Bytes() []byte {
	return id[:]
}

// Equal returns true when both identifiers are equal.
func (id BlockID) Equal(other BlockID) bool {
	return bytes.Equal(id[:], other[:])
}

func (id BlockID) String() string {
	return fmt.Sprintf("%x", id[:])[:8]
}

// Block is the interface of the unit of storage in the blockchain
type Block interface {
	encoding.Packable

	GetID() BlockID
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

// Blockchain is the interface that provides the primitives to interact with the
// blockchain.
type Blockchain interface {
	GetBlockFactory() BlockFactory

	// Store stores any representation of a data structure into a new block.
	// The implementation is responsible for any validations required.
	Store(data proto.Message, nodes mino.Node) error

	// GetBlock returns the latest block.
	GetBlock() (Block, error)

	// GetVerifiableBlock returns the latest block alongside with a proof from
	// the genesis block.
	GetVerifiableBlock() (VerifiableBlock, error)

	// Watch takes an observer that will be notified for each new block
	// definitely appended to the chain.
	Watch(ctx context.Context, obs Observer)
}
