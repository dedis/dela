package mem

import (
	"context"

	"go.dedis.ch/dela/core/tap"
	"go.dedis.ch/dela/core/tap/pool"
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
	txs map[Key]tap.Transaction
}

// NewPool creates a new service.
func NewPool() Pool {
	return Pool{
		txs: make(map[Key]tap.Transaction),
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

	s.txs[key] = tx

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

// Watch implements pool.Pool.
func (s Pool) Watch(ctx context.Context) <-chan pool.Event {
	return nil
}
