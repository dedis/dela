package blockstore

import (
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"golang.org/x/xerrors"
)

type cachedGenesis struct {
	genesis types.Genesis
	set     bool
}

// NewGenesisStore returns a new empty genesis store.
func NewGenesisStore() GenesisStore {
	return &cachedGenesis{}
}

func (s *cachedGenesis) Get() (types.Genesis, error) {
	if !s.set {
		return types.Genesis{}, xerrors.New("missing genesis")
	}

	return s.genesis, nil
}

func (s *cachedGenesis) Set(genesis types.Genesis) error {
	s.genesis = genesis
	s.set = true
	return nil
}
