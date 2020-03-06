package blockchain

import (
	"bytes"
	"context"
	fmt "fmt"
	"io"

	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	mino "go.dedis.ch/fabric/mino"
)

//go:generate protoc -I ./ --proto_path=../ --go_out=Mmino/messages.proto=go.dedis.ch/fabric/mino:. ./messages.proto

// Roster is a set of identifiable addresses.
type Roster interface {
	io.WriterTo

	GetAddresses() []*mino.Address
	GetPublicKeys() []crypto.PublicKey
	GetConodes() ([]*Conode, error)
}

type BlockID [32]byte

func NewBlockID(buffer []byte) BlockID {
	id := BlockID{}
	copy(id[:], buffer)
	return id
}

func (id BlockID) Bytes() []byte {
	return id[:]
}

func (id BlockID) Equal(other BlockID) bool {
	return bytes.Equal(id[:], other[:])
}

func (id BlockID) String() string {
	return fmt.Sprintf("%x", id[:])[:8]
}

type Block interface {
	encoding.Packable

	GetID() BlockID

	GetRoster() Roster
}

type VerifiableBlock interface {
	Block

	Verify(crypto.Verifier) error
}

// BlockFactory provides primitives to create blocks from a untrusted source.
type BlockFactory interface {
	FromVerifiable(src proto.Message, roster Roster) (Block, error)
}

// Blockchain is the interface that provides the primitives to interact with the
// blockchain.
type Blockchain interface {
	GetBlockFactory() BlockFactory

	// Store stores any representation of a data structure into a new block.
	// The implementation is responsible for any validations required.
	Store(roster Roster, data proto.Message) error

	// GetBlock returns the latest block.
	GetBlock() (Block, error)

	// GetVerifiableBlock returns the latest block alongside with a proof from
	// the genesis block.
	GetVerifiableBlock() (VerifiableBlock, error)

	// Watch takes an observer that will be notified for each new block
	// definitely appended to the chain.
	Watch(ctx context.Context, obs Observer)
}
