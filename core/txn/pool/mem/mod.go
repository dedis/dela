package mem

import (
	"context"
	"sync"

	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/pool"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

// KeyMaxLength is the maximum length of a transaction identifier.
const KeyMaxLength = 32

// Key is the key of a transaction. By definition, it expects that the
// identifier of the transaction is no more than 32 bytes long.
type Key [KeyMaxLength]byte

// Pool is a in-memory transaction pool. It only accepts transactions from a
// local client and it does not support asynchronous calls.
//
// - implements pool.Pool
type Pool struct {
	sync.Mutex
	history  map[Key]struct{}
	txs      map[Key]txn.Transaction
	gatherer pool.Gatherer
}

// NewPool creates a new service.
func NewPool() *Pool {
	return &Pool{
		history:  make(map[Key]struct{}),
		txs:      make(map[Key]txn.Transaction),
		gatherer: pool.NewSimpleGatherer(),
	}
}

// Add implements pool.Pool. It adds the transaction to the pool of waiting
// transactions.
func (s *Pool) Add(tx txn.Transaction) error {
	id := tx.GetID()
	if len(id) > KeyMaxLength {
		return xerrors.Errorf("tx identifier is too long: %d > %d", len(id), KeyMaxLength)
	}

	key := Key{}
	copy(key[:], id)

	s.Lock()

	_, found := s.history[key]
	if found {
		s.Unlock()
		return xerrors.Errorf("tx %#x already exists", key)
	}

	s.txs[key] = tx

	s.gatherer.Notify(len(s.txs), s.makeArray)

	s.Unlock()

	return nil
}

// Remove implements pool.Pool. It removes the transaction from the pool if it
// exists, otherwise it returns an error.
func (s *Pool) Remove(tx txn.Transaction) error {
	key := Key{}
	copy(key[:], tx.GetID())

	s.Lock()

	_, found := s.txs[key]
	if !found {
		s.Unlock()
		return xerrors.Errorf("transaction %#x not found", key[:])
	}

	delete(s.txs, key)

	// Keep an history of transactions to prevent duplicates to be indefinitely
	// added to the pool.
	s.history[key] = struct{}{}

	s.Unlock()

	return nil
}

// SetPlayers implements pool.Pool. It does nothing as the pool is in-memory and
// only shares the transactions to the host.
func (s *Pool) SetPlayers(mino.Players) error {
	return nil
}

// Gather implements pool.Pool. It gathers the transactions of the pool and
// return them.
func (s *Pool) Gather(ctx context.Context, cfg pool.Config) []txn.Transaction {
	s.Lock()

	if len(s.txs) >= cfg.Min {
		txs := s.makeArray()
		s.Unlock()
		return txs
	}

	s.Unlock()

	return s.gatherer.Wait(ctx, cfg)
}

func (s *Pool) makeArray() []txn.Transaction {
	txs := make([]txn.Transaction, 0, len(s.txs))
	for _, tx := range s.txs {
		txs = append(txs, tx)
	}

	return txs
}
