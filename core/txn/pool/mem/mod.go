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

// AddFilter implements pool.Pool. It adds the filter to the gatherer.
func (p *Pool) AddFilter(filter pool.Filter) {
	p.gatherer.AddFilter(filter)
}

// Add implements pool.Pool. It adds the transaction to the pool of waiting
// transactions.
func (p *Pool) Add(tx txn.Transaction) error {
	err := p.gatherer.Add(tx)
	if err != nil {
		return xerrors.Errorf("store failed: %v", err)
	}

	return nil
}

// Remove implements pool.Pool. It removes the transaction from the pool if it
// exists, otherwise it returns an error.
func (p *Pool) Remove(tx txn.Transaction) error {
	err := p.gatherer.Remove(tx)
	if err != nil {
		return xerrors.Errorf("store failed: %v", err)
	}

	return nil
}

// SetPlayers implements pool.Pool. It does nothing as the pool is in-memory and
// only shares the transactions to the host.
func (p *Pool) SetPlayers(mino.Players) error {
	return nil
}

// Gather implements pool.Pool. It gathers the transactions of the pool and
// return them.
func (p *Pool) Gather(ctx context.Context, cfg pool.Config) []txn.Transaction {
	return p.gatherer.Wait(ctx, cfg)
}

// Close implements pool.Pool. It cleans the resources of the gatherer.
func (p *Pool) Close() error {
	p.gatherer.Close()

	return nil
}

// Stats implements pool.Pool. It gets the transaction statistics.
func (p *Pool) Stats() pool.Stats {
	return p.gatherer.Stats()
}

// ResetStats implements pool.Pool. It resets the transaction statistics.
func (p *Pool) ResetStats() {
	p.gatherer.ResetStats()
}
