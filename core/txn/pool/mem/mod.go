package mem

import (
	"context"

	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/pool"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

// Pool is a in-memory transaction pool. It only accepts transactions from a
// local client and it does not support asynchronous calls.
//
// - implements pool.Pool
type Pool struct {
	gatherer pool.Gatherer
}

// NewPool creates a new service.
func NewPool() *Pool {
	return &Pool{
		gatherer: pool.NewSimpleGatherer(),
	}
}

// Len implements pool.Pool. It returns the number of transactions available in
// the pool.
func (s *Pool) Len() int {
	return s.gatherer.Len()
}

// AddFilter implements pool.Pool. It adds the filter to the gatherer.
func (s *Pool) AddFilter(filter pool.Filter) {
	s.gatherer.AddFilter(filter)
}

// Add implements pool.Pool. It adds the transaction to the pool of waiting
// transactions.
func (s *Pool) Add(tx txn.Transaction) error {
	err := s.gatherer.Add(tx)
	if err != nil {
		return xerrors.Errorf("store failed: %v", err)
	}

	return nil
}

// Remove implements pool.Pool. It removes the transaction from the pool if it
// exists, otherwise it returns an error.
func (s *Pool) Remove(tx txn.Transaction) error {
	err := s.gatherer.Remove(tx)
	if err != nil {
		return xerrors.Errorf("store failed: %v", err)
	}

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
	return s.gatherer.Wait(ctx, cfg)
}

// Close implements pool.Pool. It cleans the resources of the gatherer.
func (s *Pool) Close() error {
	s.gatherer.Close()

	return nil
}
