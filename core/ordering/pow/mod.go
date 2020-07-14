package pow

import (
	"context"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/blockchain"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/tap"
	"go.dedis.ch/dela/core/tap/pool"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/crypto"
)

type epoch struct {
	block Block
	store store.Store
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
	difficulty  int
	watcher     blockchain.Observable
	closing     chan struct{}
}

// NewService creates a new service.
func NewService(pool pool.Pool, val validation.Service, trie store.Store) *Service {
	return &Service{
		pool:        pool,
		validation:  val,
		epochs:      []epoch{{store: trie}},
		hashFactory: crypto.NewSha256Factory(),
		difficulty:  1,
		watcher:     blockchain.NewWatcher(),
		closing:     make(chan struct{}),
	}
}

// Listen implements ordering.Service.
func (s *Service) Listen() error {
	// TODO: listen for incoming mined blocks.

	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// This is the main loop to create a block. It will wait for
		// transactions at first and then try to create a new block. If another
		// peer successfully finds a correct block, it will abort and restart.
		for {
			txs := s.waitTxs(ctx)

			select {
			case <-s.closing:
				return
			case res := <-s.createBlock(ctx, txs):
				if res.err != nil {
					// Something went wrong when creating the block. The main
					// loop is stopped as this is a critical error.
					dela.Logger.Err(res.err).Msg("failed to prepare block")
					return
				}

				s.epochs = append(s.epochs, epoch{
					block: res.block,
					store: res.trie,
				})

				for _, txres := range res.block.data.GetTransactionResults() {
					err := s.pool.Remove(txres.GetTransaction())
					if err != nil {
						dela.Logger.Warn().Err(err).Msg("tx not in pool")
					}
				}

				s.watcher.Notify(ordering.Event{Index: res.block.index})

				dela.Logger.Info().Uint64("index", res.block.index).Msg("block append")
			}
		}
	}()

	return nil
}

// Close implements ordering.Service.
func (s *Service) Close() error {
	close(s.closing)
	return nil
}

// GetProof implements ordering.Service.
func (s *Service) GetProof(key []byte) (ordering.Proof, error) {
	last := s.epochs[len(s.epochs)-1]

	share, err := last.store.GetShare(key)
	if err != nil {
		return nil, err
	}

	blocks := make([]Block, len(s.epochs))
	for i, epoch := range s.epochs {
		blocks[i] = epoch.block
	}

	pr, err := NewProof(blocks, share)
	if err != nil {
		return nil, err
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

// waitTxs is a procedure to wait for transactions from the pool. It will wait
// the provided minimum amount of time before waiting for at least one
// transaction.
func (s *Service) waitTxs(ctx context.Context) []tap.Transaction {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	evts := s.pool.Watch(ctx)

	if s.pool.Len() > 0 {
		return s.pool.GetAll()
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case evt := <-evts:
			if evt.Len > 0 {
				return s.pool.GetAll()
			}
		}
	}
}

type blockResult struct {
	block Block
	trie  store.Store
	err   error
}

func (s *Service) createBlock(ctx context.Context, txs []tap.Transaction) <-chan blockResult {
	ch := make(chan blockResult, 1)

	latestEpoch := s.epochs[len(s.epochs)-1]

	go func() {
		var data validation.Data

		newTrie, err := latestEpoch.store.Stage(func(rwt store.ReadWriteTrie) error {
			var err error
			data, err = s.validation.Validate(rwt, txs)
			if err != nil {
				return err
			}

			return nil
		})

		if err != nil {
			ch <- blockResult{err: err}
			return
		}

		block, err := NewBlock(ctx, data, WithIndex(uint64(len(s.epochs))), WithRoot(newTrie.GetRoot()))

		// The result will contain either a valid block and trie, or the error
		// that prevent the creation.
		ch <- blockResult{
			block: block,
			trie:  newTrie,
			err:   err,
		}
	}()

	return ch
}
