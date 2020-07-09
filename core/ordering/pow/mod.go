package pow

import (
	"context"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/tap"
	"go.dedis.ch/dela/core/tap/pool"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/crypto"
	"golang.org/x/xerrors"
)

type epoch struct {
	block Block
	trie  store.Trie
}

// Service is an ordering service powered by a Proof-of-Work consensus
// algorithm.
//
// - implements ordering.Service
type Service struct {
	pool        pool.Pool
	validation  validation.Service
	ch          <-chan Block
	epochs      []epoch
	hashFactory crypto.HashFactory
	difficulty  int
}

// NewService creates a new service.
func NewService(pool pool.Pool, val validation.Service) Service {
	return Service{
		pool:        pool,
		validation:  val,
		ch:          make(<-chan Block),
		epochs:      make([]epoch, 0),
		hashFactory: crypto.NewSha256Factory(),
		difficulty:  1,
	}
}

// Listen implements ordering.Service.
func (s Service) Listen() error {
	// TODO: listen for incoming mined blocks.

	go func() {
		// This is the main loop to create a block. It will wait for
		// transactions at first and then try to create a new block. If another
		// peer successfully finds a correct block, it will abort and restart.
		for {
			ctx, cancel := context.WithCancel(context.Background())

			txs := s.waitTxs(ctx)

			var e epoch
			select {
			case block := <-s.ch:
				trie, err := s.validateBlock(block)
				if err != nil {
					dela.Logger.Err(err).Msg("invalid block")
				}

				// Stop any work in progress to create a new block.
				cancel()

				e.block = block
				e.trie = trie
			case res := <-s.createBlock(ctx, txs):
				if res.err != nil {
					dela.Logger.Err(res.err).Msg("failed to prepare block")
					continue
				}

				e.block = res.block
				e.trie = res.trie
			}

			s.epochs = append(s.epochs, e)
		}
	}()

	return nil
}

// GetBlock implements ordering.Service. It returns the block at the given index
// if it exists.
func (s Service) GetBlock(index uint64) (ordering.Block, error) {
	idx := int(index)
	if idx >= len(s.epochs) {
		return nil, xerrors.Errorf("unknown block at index '%d'", index)
	}

	return s.epochs[index].block, nil
}

// Watch implements ordering.Service.
func (s Service) Watch(ctx context.Context) <-chan ordering.Event {
	return nil
}

// waitTxs is a procedure to wait for transactions from the pool. It will wait
// the provided minimum amount of time before waiting for at least one
// transaction.
func (s Service) waitTxs(ctx context.Context) []tap.Transaction {
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

type result struct {
	block Block
	trie  store.Trie
	err   error
}

func (s Service) createBlock(ctx context.Context, txs []tap.Transaction) <-chan result {
	ch := make(chan result, 1)

	latestEpoch := s.epochs[len(s.epochs)-1]

	go func() {
		var finalBlock Block

		newTrie, err := latestEpoch.trie.Stage(func(rwt store.ReadWriteTrie) error {
			data, err := s.validation.Validate(rwt, txs)
			if err != nil {
				return err
			}

			block := NewBlock(uint64(len(s.epochs)), 0, data)

			finalBlock, err = block.Prepare(ctx, s.hashFactory, s.difficulty)
			if err != nil {
				return err
			}

			return nil
		})

		ch <- result{
			block: finalBlock,
			trie:  newTrie,
			err:   err,
		}
	}()

	return ch
}

func (s Service) validateBlock(b Block) (store.Trie, error) {
	return nil, nil
}
