package blockstore

import (
	"context"
	"errors"

	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/hashtree"
)

// ErrNoBlock is the error message returned when the block is unknown.
var ErrNoBlock = errors.New("no block")

// TreeCache is a cache to store a tree that needs to be accessed in different
// places.
type TreeCache interface {
	Get() hashtree.Tree

	Set(hashtree.Tree)
}

// GenesisStore is the interface to store and get the genesis block. It is left
// to the implementation to persist it.
type GenesisStore interface {
	// Get must return the genesis block if it is set, otherwise an error.
	Get() (types.Genesis, error)

	// Set must set the genesis block.
	Set(types.Genesis) error

	// Exists returns true if the genesis is already set.
	Exists() bool
}

// BlockStore is the interface to store and get blocks.
type BlockStore interface {
	// Len must return the length of the store.
	Len() uint64

	// Store must store the block link only if it matches the latest link,
	// otherwise it must return an error.
	Store(types.BlockLink) error

	// Get must return the block link associated to the digest, or an error.
	Get(id types.Digest) (types.BlockLink, error)

	// GetByIndex return the block link associated to the index, or an error.
	GetByIndex(index uint64) (types.BlockLink, error)

	// GetChain returns a chain of the blocks. It can be used to prove the
	// integrity of the last block from the genesis.
	GetChain() (types.Chain, error)

	// Last must return the latest block link in the store.
	Last() (types.BlockLink, error)

	// Watch returns a channel that is filled with new block links. The is
	// closed as soon as the context is done.
	Watch(context.Context) <-chan types.BlockLink

	// WithTx returns a block store that is using the transaction to perform
	// operations on the database.
	WithTx(store.Transaction) BlockStore
}
