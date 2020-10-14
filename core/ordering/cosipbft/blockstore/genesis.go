// This file contains the implementations of a genesis store. An in-memory and a
// persistent implementation are available.
//
// Documentation Last Review: 13.10.2020
//

package blockstore

import (
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
	"golang.org/x/xerrors"
)

var genesisBucket = []byte("blockstore-genesis")
var genesisKey = []byte("block")

// CachedGenesis is a store to keep a genesis block in cache.
//
// - implements blockstore.GenesisStore
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

// PersistentGenesisCache is a store to set a genesis block on disk. It also
// provides a function to load the block from the disk that will then stay in
// memory.
//
// - implements blockstore.GenesisStore
type PersistentGenesisCache struct {
	*cachedGenesis

	db      kv.DB
	context serde.Context
	fac     serde.Factory
}

// NewGenesisDiskStore creates a new store that will load or set the genesis
// block using the given database.
func NewGenesisDiskStore(db kv.DB, fac serde.Factory) PersistentGenesisCache {
	return PersistentGenesisCache{
		cachedGenesis: &cachedGenesis{},
		db:            db,
		context:       json.NewContext(),
		fac:           fac,
	}
}

// Load tries to read the genesis block in the database, and set it to memory
// only if it exists. It returns an error if the database cannot be read, or the
// value is malformed.
func (s PersistentGenesisCache) Load() error {
	return s.db.View(func(tx kv.ReadableTx) error {
		bucket := tx.GetBucket(genesisBucket)
		if bucket == nil {
			// Nothing in the database, so the load is successful but no genesis
			// is set.
			return nil
		}

		data := bucket.Get(genesisKey)

		msg, err := s.fac.Deserialize(s.context, data)
		if err != nil {
			return xerrors.Errorf("malformed value: %v", err)
		}

		genesis, ok := msg.(types.Genesis)
		if !ok {
			return xerrors.Errorf("unsupported message '%T'", msg)
		}

		s.set = true
		s.genesis = genesis

		return nil
	})
}

// Set implements blockstore.GenesisStore. It writes the genesis block in
// disk, and then in memory for fast access.
func (s PersistentGenesisCache) Set(genesis types.Genesis) error {
	if !s.set {
		data, err := genesis.Serialize(s.context)
		if err != nil {
			return xerrors.Errorf("failed to serialize genesis: %v", err)
		}

		err = s.db.Update(func(tx kv.WritableTx) error {
			bucket, err := tx.GetBucketOrCreate(genesisBucket)
			if err != nil {
				return xerrors.Errorf("bucket: %v", err)
			}

			err = bucket.Set(genesisKey, data)
			if err != nil {
				return xerrors.Errorf("while writing to bucket: %v", err)
			}

			return nil
		})

		if err != nil {
			return xerrors.Errorf("store failed: %v", err)
		}
	}

	return s.cachedGenesis.Set(genesis)
}
