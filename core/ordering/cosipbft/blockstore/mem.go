// This file contains an in-memory implementation of a block store.
//
// Documentation Last Review: 13.10.2020
//

package blockstore

import (
	"context"
	"sync"

	"github.com/rs/zerolog"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/core"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/store"
	"golang.org/x/xerrors"
)

// sizeWarnLimit defines the limit after which a warning is emitted every time
// the queue keeps growing.
const sizeWarnLimit = 100

// InMemory is a block store that only stores the block in-memory which means
// they won't persist.
//
// - implements blockstore.BlockStore
type InMemory struct {
	sync.Mutex
	blocks  []types.BlockLink
	watcher core.Observable
	withTx  bool
}

// NewInMemory returns a new empty in-memory block store.
func NewInMemory() *InMemory {
	return &InMemory{
		blocks:  make([]types.BlockLink, 0),
		watcher: core.NewWatcher(),
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

		if latest.GetTo() != link.GetFrom() {
			return xerrors.Errorf("mismatch link '%v' != '%v'", link.GetFrom(), latest.GetTo())
		}
	}

	s.blocks = append(s.blocks, link)

	if !s.withTx {
		// When the store is using a database transaction, it will delay the
		// notification until the commit.
		s.watcher.Notify(link)
	}

	return nil
}

// Get implements blockstore.BlockStore. It returns the block link associated to
// the digest if it exists, otherwise it returns an error.
func (s *InMemory) Get(id types.Digest) (types.BlockLink, error) {
	s.Lock()
	defer s.Unlock()

	for _, link := range s.blocks {
		if link.GetTo() == id {
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

// GetChain implements blockstore.BlockStore. It returns the chain to the latest
// block.
func (s *InMemory) GetChain() (types.Chain, error) {
	s.Lock()
	defer s.Unlock()

	num := len(s.blocks) - 1

	if num < 0 {
		return nil, xerrors.New("store is empty")
	}

	prevs := make([]types.Link, num)
	for i, block := range s.blocks[:num] {
		prevs[i] = block.Reduce()
	}

	return types.NewChain(s.blocks[num], prevs), nil
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
	obs := newObserver(ctx, s.watcher)

	return obs.ch
}

// WithTx implements blockstore.BlockStore. It returns a new store that will
// apply the list of blocks at the end of the transaction.
func (s *InMemory) WithTx(txn store.Transaction) BlockStore {
	store := &InMemory{
		blocks:  append([]types.BlockLink{}, s.blocks...),
		watcher: s.watcher,
		withTx:  true,
	}

	from := len(store.blocks)

	txn.OnCommit(func() {
		s.Lock()
		s.blocks = store.blocks
		s.withTx = false

		newBlocks := append([]types.BlockLink{}, s.blocks[from:]...)
		s.Unlock()

		for _, link := range newBlocks {
			s.watcher.Notify(link)
		}
	})

	return store
}

// Observer is an observer that can be added to store watcher. It will announce
// the blocks in order and without blocking the watcher even if the listener is
// not actively emptying the queue.
//
// - implements core.Observer.
type observer struct {
	sync.Mutex

	logger  zerolog.Logger
	buffer  []types.BlockLink
	running bool
	closed  bool
	working sync.WaitGroup
	ch      chan types.BlockLink
}

func newObserver(ctx context.Context, watcher core.Observable) *observer {
	// This channel must have a buffer of size 1, no more no less, in order to
	// preserve the ordering of the events but also to prevent the observer
	// buffer to be used when the channel is correctly listened to.
	ch := make(chan types.BlockLink, 1)

	obs := &observer{
		logger: dela.Logger,
		ch:     ch,
	}

	watcher.Add(obs)

	go func() {
		<-ctx.Done()
		watcher.Remove(obs)
		obs.close()
	}()

	return obs
}

// NotifyCallback implements core.Observer. It pushes the event to the channel
// if it is free, otherwise it fills a buffer and waits for the channel to be
// drained.
func (obs *observer) NotifyCallback(evt interface{}) {
	obs.Lock()
	defer obs.Unlock()

	if obs.closed {
		return
	}

	if obs.running {
		// We know the channel is busy so it goes directly to the buffer.
		obs.buffer = append(obs.buffer, evt.(types.BlockLink))
		obs.checkSize()
		return
	}

	select {
	case obs.ch <- evt.(types.BlockLink):

	default:
		// The buffer size is not controlled as we assume the event will be read
		// shortly by the caller.
		obs.buffer = append(obs.buffer, evt.(types.BlockLink))

		obs.checkSize()

		obs.running = true

		obs.working.Add(1)

		go obs.pushAndWait()
	}
}

func (obs *observer) checkSize() {
	if len(obs.buffer) >= sizeWarnLimit {
		obs.logger.Warn().
			Int("size", len(obs.buffer)).
			Msg("observer queue is growing unexpectedly")
	}
}

func (obs *observer) pushAndWait() {
	defer obs.working.Done()

	for {
		obs.Lock()

		if len(obs.buffer) == 0 {
			obs.running = false
			obs.Unlock()
			return
		}

		msg := obs.buffer[0]
		obs.buffer = obs.buffer[1:]

		obs.Unlock()

		// Wait for the channel to be available to writings.
		obs.ch <- msg
	}
}

func (obs *observer) close() {
	obs.Lock()

	obs.closed = true
	obs.running = false
	obs.buffer = nil

	// Drain message in transit to close the channel properly.
	select {
	case <-obs.ch:
	default:
	}

	close(obs.ch)

	obs.Unlock()

	obs.working.Wait()
}
