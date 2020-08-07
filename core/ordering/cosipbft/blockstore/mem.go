package blockstore

import (
	"context"
	"sync"

	"go.dedis.ch/dela/blockchain"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"golang.org/x/xerrors"
)

// InMemory is a block store that only stores the block in-memory which means
// they won't persist.
//
// - implements blockstore.BlockStore
type InMemory struct {
	sync.Mutex
	blocks  []types.BlockLink
	watcher blockchain.Observable
}

// NewInMemory returns a new empty in-memory block store.
func NewInMemory() *InMemory {
	return &InMemory{
		blocks:  make([]types.BlockLink, 0),
		watcher: blockchain.NewWatcher(),
	}
}

// Len implements blockstore.BlockStore. It returns the length of the store.
func (s *InMemory) Len() uint64 {
	s.Lock()
	defer s.Unlock()

	return uint64(len(s.blocks))
}

// Store implements blockstore.BlockStore. It stores the block only if the link
// matches the latest block.
func (s *InMemory) Store(link types.BlockLink) error {
	s.Lock()
	defer s.Unlock()

	if len(s.blocks) > 0 {
		latest := s.blocks[len(s.blocks)-1]

		if latest.GetTo().GetHash() != link.GetFrom() {
			return xerrors.Errorf("mismatch link '%v' != '%v'",
				link.GetFrom(), latest.GetTo().GetHash())
		}
	}

	s.blocks = append(s.blocks, link)

	s.watcher.Notify(link)

	return nil
}

// Get implements blockstore.BlockStore. It returns the block link associated to
// the digest if it exists, otherwise it returns an error.
func (s *InMemory) Get(id types.Digest) (types.BlockLink, error) {
	s.Lock()
	defer s.Unlock()

	for _, link := range s.blocks {
		if link.GetTo().GetHash() == id {
			return link, nil
		}
	}

	return nil, xerrors.Errorf("block not found: %w", ErrNoBlock)
}

// GetByIndex implements blockstore.BlockStore. It returns the block associated
// to the index if it exists.
func (s *InMemory) GetByIndex(index uint64) (types.BlockLink, error) {
	s.Lock()
	defer s.Unlock()

	if int(index) >= len(s.blocks) {
		return nil, xerrors.Errorf("block not found: %w", ErrNoBlock)
	}

	return s.blocks[index], nil
}

// Last implements blockstore.BlockStore. It returns the latest block of the
// store.
func (s *InMemory) Last() (types.BlockLink, error) {
	s.Lock()
	defer s.Unlock()

	if len(s.blocks) == 0 {
		return nil, xerrors.Errorf("store empty: %w", ErrNoBlock)
	}

	return s.blocks[len(s.blocks)-1], nil
}

// Watch implements blockstore.BlockStore. It returns a channel populated with
// new blocks.
func (s *InMemory) Watch(ctx context.Context) <-chan types.BlockLink {
	obs := observer{ch: make(chan types.BlockLink, 1)}
	s.watcher.Add(obs)

	go func() {
		<-ctx.Done()
		s.watcher.Remove(obs)
		close(obs.ch)
	}()

	return obs.ch
}

type observer struct {
	ch chan types.BlockLink
}

func (obs observer) NotifyCallback(evt interface{}) {
	obs.ch <- evt.(types.BlockLink)
}
