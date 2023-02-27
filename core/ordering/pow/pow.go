// Package pow implements a Proof-of-Work ordering service. This implementation
// very naive and only support one single node. It demonstrates a permissionless
// blockchain.
//
// TODO: later improve or remove
package pow

import (
	"context"
	"sync"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/core"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/hashtree"
	"go.dedis.ch/dela/core/txn/pool"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/crypto"
	"golang.org/x/xerrors"
)

type epoch struct {
	block Block
	store hashtree.Tree
}

// Service is an ordering service powered by a Proof-of-Work consensus
// algorithm.
//
// - implements ordering.Service
type Service struct {
	pool        pool.Pool
	validation  validation.Service
	epochs      []epoch
	hashFactory crypto.HashFactory
	difficulty  uint
	watcher     core.Observable
	closing     chan struct{}
	closed      sync.WaitGroup
}

// NewService creates a new service.
func NewService(pool pool.Pool, val validation.Service, trie hashtree.Tree) *Service {
	return &Service{
		pool:        pool,
		validation:  val,
		epochs:      []epoch{{store: trie}},
		hashFactory: crypto.NewSha256Factory(),
		difficulty:  1,
		watcher:     core.NewWatcher(),
	}
}

// Listen implements ordering.Service.
func (s *Service) Listen() error {
	if s.closing != nil {
		return xerrors.New("service already started")
	}

	s.closing = make(chan struct{})
	s.closed = sync.WaitGroup{}
	s.closed.Add(1)

	go func() {
		defer s.closed.Done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// This is the main loop to create a block. It will wait for
		// transactions at first and then try to create a new block. If another
		// peer successfully finds a correct block, it will abort and restart.
		for {
			errCh := make(chan error, 1)
			go func() {
				errCh <- s.createBlock(ctx)
			}()

			select {
			case <-s.closing:
				// This will cancel the context thus stopping the go routine.
				return
			case err := <-errCh:
				if err != nil {
					// Something went wrong when creating the block. The main
					// loop is stopped as this is a critical error.
					dela.Logger.Err(err).Msg("failed to create block")
					return
				}

			}
		}
	}()

	return nil
}

// Stop implements ordering.Service.
func (s *Service) Stop() error {
	if s.closing == nil {
		return xerrors.New("service not started")
	}

	close(s.closing)
	s.closed.Wait()

	s.closing = nil

	return nil
}

// GetProof implements ordering.Service.
func (s *Service) GetProof(key []byte) (ordering.Proof, error) {
	last := s.epochs[len(s.epochs)-1]

	path, err := last.store.GetPath(key)
	if err != nil {
		return nil, xerrors.Errorf("couldn't read share: %v", err)
	}

	blocks := make([]Block, len(s.epochs))
	for i, epoch := range s.epochs {
		blocks[i] = epoch.block
	}

	pr, err := NewProof(blocks, path)
	if err != nil {
		return nil, xerrors.Errorf("couldn't create proof: %v", err)
	}

	return pr, nil
}

// Watch implements ordering.Service.
func (s *Service) Watch(ctx context.Context) <-chan ordering.Event {
	events := make(chan ordering.Event, 1)

	obs := observer{events: events}
	s.watcher.Add(obs)

	go func() {
		<-ctx.Done()
		s.watcher.Remove(obs)
	}()

	return events
}

func (s *Service) createBlock(ctx context.Context) error {
	// Wait for at least one transaction before creating a block.
	txs := s.pool.Gather(ctx, pool.Config{Min: 1})

	if ctx.Err() != nil {
		// Context is closed so we don't proceed in the block creation.
		return nil
	}

	latestEpoch := s.epochs[len(s.epochs)-1]

	var data validation.Result
	newTrie, err := latestEpoch.store.Stage(func(rwt store.Snapshot) error {
		var err error
		data, err = s.validation.Validate(rwt, txs)
		if err != nil {
			return xerrors.Errorf("failed to validate: %v", err)
		}

		return nil
	})

	if err != nil {
		return xerrors.Errorf("couldn't stage store: %v", err)
	}

	block, err := NewBlock(
		ctx,
		data,
		WithIndex(uint64(len(s.epochs))),
		WithRoot(newTrie.GetRoot()),
		WithDifficulty(s.difficulty),
	)
	if err != nil {
		return xerrors.Errorf("couldn't create block: %v", err)
	}

	s.epochs = append(s.epochs, epoch{
		block: block,
		store: newTrie,
	})

	for _, txres := range block.data.GetTransactionResults() {
		s.pool.Remove(txres.GetTransaction())
	}

	s.watcher.Notify(ordering.Event{Index: block.index})

	dela.Logger.Trace().Uint64("index", block.index).Msg("block append")

	return nil
}
