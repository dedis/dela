package blockstore

import (
	"errors"

	"go.dedis.ch/dela/core/ordering/cosipbft/types"
)

// ErrNoBlock is the error message returned when the block is unknown.
var ErrNoBlock = errors.New("no block")

// GenesisStore is the interface to store and get the genesis block. It is left
// to the implementation to persist it.
type GenesisStore interface {
	// Get must return the genesis block if it is set, otherwise an error.
	Get() (types.Genesis, error)

	// Set must set the genesis block.
	Set(types.Genesis) error
}

// BlockStore is the interface to store and get blocks.
type BlockStore interface {
	// Len must return the length of the store.
	Len() int

	// Store must store the block link only if it matches the latest link,
	// otherwise it must return an error.
	Store(types.BlockLink) error

	// Get must return the block link associated to the digest, otherwise an
	// error.
	Get(id types.Digest) (types.BlockLink, error)

	// Last must return the latest block link in the store.
	Last() (types.BlockLink, error)
}
