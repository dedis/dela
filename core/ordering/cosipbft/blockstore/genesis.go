package blockstore

import (
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"golang.org/x/xerrors"
)

// CachedGenesis is a store to keep a genesis block in cache.
//
// - implements blockstore.GenesisStore
//
// TODO: persistent storage
type cachedGenesis struct {
	genesis types.Genesis
	set     bool
}

// NewGenesisStore returns a new empty genesis store.
func NewGenesisStore() GenesisStore {
	return &cachedGenesis{}
}

// Get implements blockstore.GenesisStore. It returns the genesis block if it is
// set, otherwise it returns an error.
func (s *cachedGenesis) Get() (types.Genesis, error) {
	if !s.set {
		return types.Genesis{}, xerrors.New("missing genesis block")
	}

	return s.genesis, nil
}

// Set implements blockstore.GenesisStore. It sets the genesis block only if the
// cache is empty, otherwise it returns an error.
func (s *cachedGenesis) Set(genesis types.Genesis) error {
	if s.set {
		return xerrors.New("genesis block is already set")
	}

	s.genesis = genesis
	s.set = true
	return nil
}

// Exists implements blockstore.GenesisStore. It returns true if the genesis
// block is set.
func (s *cachedGenesis) Exists() bool {
	return s.set
}
