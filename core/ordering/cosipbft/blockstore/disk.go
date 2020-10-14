// This file contains the implementation of a persistent block store. It stores
// the blocks and their links to a key/value database.
//
// Documentation Last Review: 13.10.2020
//

package blockstore

import (
	"context"
	"encoding/binary"
	"sync"

	"go.dedis.ch/dela/core"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
	"golang.org/x/xerrors"
)

type cachedData struct {
	sync.Mutex

	length  uint64
	last    types.BlockLink
	indices map[types.Digest]uint64
}

// InDisk is a persistent storage implementation for the blocks.
//
// - implements blockstore.BlockStore
type InDisk struct {
	*cachedData

	db      kv.DB
	bucket  []byte
	context serde.Context
	fac     types.LinkFactory
	watcher core.Observable

	txn store.Transaction
}

// NewDiskStore creates a new persistent storage.
func NewDiskStore(db kv.DB, fac types.LinkFactory) *InDisk {
	return &InDisk{
		db:      db,
		bucket:  []byte("blocks"),
		context: json.NewContext(),
		fac:     fac,
		watcher: core.NewWatcher(),
		cachedData: &cachedData{
			indices: make(map[types.Digest]uint64),
		},
	}
}

// Len implements blockstore.BlockStore. It returns the number of blocks stored
// in the database.
func (s *InDisk) Len() uint64 {
	s.Lock()
	defer s.Unlock()

	return s.length
}

// Load reads the database to rebuild the cache.
func (s *InDisk) Load() error {
	s.Lock()
	defer s.Unlock()

	return s.doView(func(tx kv.ReadableTx) error {
		bucket := tx.GetBucket(s.bucket)
		if bucket == nil {
			return nil
		}

		err := bucket.Scan([]byte{}, func(key, value []byte) error {
			link, err := s.fac.BlockLinkOf(s.context, value)
			if err != nil {
				return xerrors.Errorf("malformed block: %v", err)
			}

			s.length++
			s.last = link
			s.indices[link.GetBlock().GetHash()] = link.GetBlock().GetIndex()

			return nil
		})

		if err != nil {
			return xerrors.Errorf("while scanning: %v", err)
		}

		return nil
	})
}

// Store implements blockstore.BlockStore. It stores the link in the database if
// it matches the latest link.
func (s *InDisk) Store(link types.BlockLink) error {
	s.Lock()
	last := s.last
	s.Unlock()

	if last != nil && last.GetTo() != link.GetFrom() {
		return xerrors.Errorf("mismatch digests '%v' (new) != '%v' (last)",
			link.GetFrom(), last.GetTo())
	}

	data, err := link.Serialize(s.context)
	if err != nil {
		return xerrors.Errorf("failed to serialize: %v", err)
	}

	return s.doUpdate(func(tx kv.WritableTx) error {
		bucket, err := tx.GetBucketOrCreate(s.bucket)
		if err != nil {
			return xerrors.Errorf("bucket failed: %v", err)
		}

		index := link.GetBlock().GetIndex()

		key := s.makeKey(index)

		err = bucket.Set(key, data)
		if err != nil {
			return xerrors.Errorf("while writing: %v", err)
		}

		tx.OnCommit(func() {
			s.Lock()

			s.length++
			s.last = link
			s.indices[link.GetBlock().GetHash()] = index

			s.Unlock()

			s.watcher.Notify(link)
		})

		return nil
	})
}

// Get implements blockstore.BlockStore. It loads the block with the given
// identifier if it exists, otherwise it returns an error.
func (s *InDisk) Get(id types.Digest) (types.BlockLink, error) {
	s.Lock()
	index, found := s.indices[id]
	s.Unlock()

	if !found {
		return nil, xerrors.Errorf("'%v' not found: %w", id, ErrNoBlock)
	}

	return s.GetByIndex(index)
}

// GetByIndex implements blockstore.BlockStore. It returns the block associated
// to the index if it exists, otherwise it returns an error.
func (s *InDisk) GetByIndex(index uint64) (link types.BlockLink, err error) {
	key := s.makeKey(index)

	err = s.doView(func(tx kv.ReadableTx) error {
		bucket := tx.GetBucket(s.bucket)

		value := bucket.Get(key)

		if len(value) == 0 {
			return xerrors.Errorf("index %d not found: %w", index, ErrNoBlock)
		}

		var err error
		link, err = s.fac.BlockLinkOf(s.context, value)
		if err != nil {
			return xerrors.Errorf("malformed block: %v", err)
		}

		return nil
	})

	return
}

// GetChain implements blockstore.Blockstore. It returns a chain to the latest
// block.
func (s *InDisk) GetChain() (types.Chain, error) {
	s.Lock()
	length := s.length
	s.Unlock()

	if length == 0 {
		return nil, xerrors.New("store is empty")
	}

	prevs := make([]types.Link, length-1)

	var chain types.Chain

	err := s.doView(func(tx kv.ReadableTx) error {
		bucket := tx.GetBucket(s.bucket)

		i := uint64(0)
		err := bucket.Scan([]byte{}, func(key, value []byte) error {
			link, err := s.fac.BlockLinkOf(s.context, value)
			if err != nil {
				return xerrors.Errorf("block malformed: %v", err)
			}

			if i >= length-1 {
				chain = types.NewChain(link, prevs)
				return nil
			}

			prevs[i] = link.Reduce()
			i++

			return nil
		})

		if err != nil {
			return xerrors.Errorf("while scanning: %v", err)
		}

		return nil
	})

	if err != nil {
		return nil, xerrors.Errorf("while reading database: %v", err)
	}

	return chain, nil
}

// Last implements blockstore.BlockStore. It returns the last block stored in
// the database.
func (s *InDisk) Last() (types.BlockLink, error) {
	s.Lock()
	defer s.Unlock()

	if s.length == 0 {
		return nil, xerrors.Errorf("store is empty: %w", ErrNoBlock)
	}

	return s.last, nil
}

// Watch implements blockstore.BlockStore. It returns a channel populated with
// new blocks stored.
func (s *InDisk) Watch(ctx context.Context) <-chan types.BlockLink {
	obs := newObserver(ctx, s.watcher)

	return obs.ch
}

// WithTx implements blockstore.BlockStore. It returns a store that will use the
// transaction for the operations on the database.
func (s *InDisk) WithTx(txn store.Transaction) BlockStore {
	store := &InDisk{
		db:         s.db,
		bucket:     s.bucket,
		context:    s.context,
		fac:        s.fac,
		watcher:    s.watcher,
		cachedData: s.cachedData,
		txn:        txn,
	}

	return store
}

func (s *InDisk) doUpdate(fn func(tx kv.WritableTx) error) error {
	if s.txn != nil {
		tx, ok := s.txn.(kv.WritableTx)
		if !ok {
			return xerrors.Errorf("transaction '%T' is not writable", s.txn)
		}

		return fn(tx)
	}

	return s.db.Update(fn)
}

func (s *InDisk) doView(fn func(tx kv.ReadableTx) error) error {
	if s.txn != nil {
		tx, ok := s.txn.(kv.ReadableTx)
		if !ok {
			return xerrors.Errorf("transaction '%T' is not readable", s.txn)
		}

		return fn(tx)
	}

	return s.db.View(fn)
}

func (s *InDisk) makeKey(index uint64) []byte {
	key := make([]byte, 8)
	binary.LittleEndian.PutUint64(key, index)

	return key
}
