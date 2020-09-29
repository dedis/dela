// Package gossip implements a transaction pool that is using a gossip protocol
// to spread the transactions to other participants.
package gossip

import (
	"context"

	"github.com/rs/zerolog"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/pool"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/gossip"
	"golang.org/x/xerrors"
)

// Pool is a transaction pool that is using gossip to send the transactions to
// the other participants.
//
// - implements pool.Pool
type Pool struct {
	logger   zerolog.Logger
	actor    gossip.Actor
	gatherer pool.Gatherer
	closing  chan struct{}
}

// NewPool creates a new empty pool and starts to gossip incoming transaction.
func NewPool(gossiper gossip.Gossiper) (*Pool, error) {
	actor, err := gossiper.Listen()
	if err != nil {
		return nil, xerrors.Errorf("failed to listen: %v", err)
	}

	p := &Pool{
		logger:   dela.Logger,
		actor:    actor,
		gatherer: pool.NewSimpleGatherer(),
		closing:  make(chan struct{}),
	}

	go p.listenRumors(gossiper.Rumors())

	return p, nil
}

// SetPlayers implements pool.Pool. It sets the list of participants the
// transactions should be gossiped to.
func (p *Pool) SetPlayers(players mino.Players) error {
	p.actor.SetPlayers(players)
	return nil
}

// AddFilter implements pool.Pool. It adds the filter to the gatherer.
func (p *Pool) AddFilter(filter pool.Filter) {
	p.gatherer.AddFilter(filter)
}

// Len implements pool.Pool. It returns the number of transactions available in
// the pool.
func (p *Pool) Len() int {
	return p.gatherer.Len()
}

// Add implements pool.Pool. It adds the transaction to the pool and gossips it
// to other participants.
func (p *Pool) Add(tx txn.Transaction) error {
	err := p.gatherer.Add(tx)
	if err != nil {
		return xerrors.Errorf("store failed: %v", err)
	}

	err = p.actor.Add(tx)
	if err != nil {
		return xerrors.Errorf("failed to gossip tx: %v", err)
	}

	return nil
}

// Remove implements pool.Pool. It removes the transaction from the pool.
func (p *Pool) Remove(tx txn.Transaction) error {
	err := p.gatherer.Remove(tx)
	if err != nil {
		return xerrors.Errorf("store failed: %v", err)
	}

	return nil
}

// Gather implements pool.Pool. It blocks until the pool has enough transactions
// according to the configuration and then returns the transactions.
func (p *Pool) Gather(ctx context.Context, cfg pool.Config) []txn.Transaction {
	return p.gatherer.Wait(ctx, cfg)
}

// Close stops the gossiper and terminate the routine that listens for rumors.
func (p *Pool) Close() error {
	p.gatherer.Close()

	close(p.closing)

	err := p.actor.Close()
	if err != nil {
		return xerrors.Errorf("failed to close gossiper: %v", err)
	}

	return nil
}

func (p *Pool) listenRumors(ch <-chan gossip.Rumor) {
	for {
		select {
		case rumor := <-ch:
			tx, ok := rumor.(txn.Transaction)
			if ok {
				err := p.gatherer.Add(tx)
				if err != nil {
					p.logger.Debug().Err(err).Msg("failed to add transaction")
				}
			}
		case <-p.closing:
			return
		}
	}
}
