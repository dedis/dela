package blockstore

import (
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
)

const ErrBlockUnknown = "block is unknown"

type GenesisStore interface {
	Get() (types.Genesis, error)
	Set(types.Genesis) error
}

type BlockStore interface {
	Len() int
	Store(types.BlockLink) error
	Get(id []byte) (types.BlockLink, error)
	Last() (types.BlockLink, error)
}
