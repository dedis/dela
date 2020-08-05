package mem

import (
	"context"

	"go.dedis.ch/dela/blockchain"
	"go.dedis.ch/dela/core/tap"
	"go.dedis.ch/dela/core/tap/pool"
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
	history map[Key]struct{}
	txs     map[Key]tap.Transaction
	watcher blockchain.Observable
}

// NewPool creates a new service.
func NewPool() Pool {
	return Pool{
		history: make(map[Key]struct{}),
		txs:     make(map[Key]tap.Transaction),
		watcher: blockchain.NewWatcher(),
	}
}

// Len implements pool.Pool. It returns the current length of the pool.
func (s Pool) Len() int {
	return len(s.txs)
}

// Add implements pool.Pool. It adds the transaction to the pool of waiting
// transactions.
func (s Pool) Add(tx tap.Transaction) error {
	id := tx.GetID()
	if len(id) > KeyMaxLength {
		return xerrors.Errorf("tx identifier is too long: %d > %d", len(id), KeyMaxLength)
	}

	key := Key{}
	copy(key[:], id)

	_, found := s.history[key]
	if found {
		return xerrors.Errorf("tx %#x already exists", key)
	}

	s.txs[key] = tx

	s.watcher.Notify(pool.Event{Len: len(s.txs)})

	return nil
}

// Remove implements pool.Pool. It removes the transaction from the pool if it
// exists, otherwise it returns an error.
func (s Pool) Remove(tx tap.Transaction) error {
	key := Key{}
	copy(key[:], tx.GetID())

	_, found := s.txs[key]
	if !found {
		return xerrors.Errorf("transaction %#x not found", key[:])
	}

	delete(s.txs, key)

	// Keep an history of transactions to prevent duplicates to be indefinitely
	// added to the pool.
	s.history[key] = struct{}{}

	s.watcher.Notify(pool.Event{Len: len(s.txs)})

	return nil
}

// GetAll implements pool.Pool. It returns all the waiting transactions in
// the pool.
func (s Pool) GetAll() []tap.Transaction {
	txs := make([]tap.Transaction, 0, len(s.txs))
	for _, tx := range s.txs {
		txs = append(txs, tx)
	}

	return txs
}

// SetPlayers implements pool.Pool. It does nothing as the pool is in-memory and
// only shares the transactions to the host.
func (s Pool) SetPlayers(mino.Players) error {
	return nil
}

// Watch implements pool.Pool.
func (s Pool) Watch(ctx context.Context) <-chan pool.Event {
	ch := make(chan pool.Event, 1)

	obs := observer{ch: ch}
	s.watcher.Add(obs)

	go func() {
		<-ctx.Done()
		s.watcher.Remove(obs)
		close(ch)
	}()

	return ch
}

type observer struct {
	ch chan pool.Event
}

func (obs observer) NotifyCallback(evt interface{}) {
	obs.ch <- evt.(pool.Event)
}
