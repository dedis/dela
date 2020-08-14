// Package gossip implements a transaction pool that is using a gossip protocol
// to spread the transactions to other participants.
package gossip

import (
	"context"
	"sync"

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
	sync.Mutex
	actor    gossip.Actor
	bag      map[string]txn.Transaction
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
		actor:    actor,
		bag:      make(map[string]txn.Transaction),
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

// Add implements pool.Pool. It adds the transaction to the pool and gossips it
// to other participants.
func (p *Pool) Add(tx txn.Transaction) error {
	err := p.actor.Add(tx)
	if err != nil {
		return xerrors.Errorf("failed to gossip tx: %v", err)
	}

	p.setTx(tx)

	return nil
}

// Remove implements pool.Pool. It removes the transaction from the pool.
func (p *Pool) Remove(tx txn.Transaction) error {
	p.Lock()
	delete(p.bag, string(tx.GetID()))
	p.Unlock()

	return nil
}

// Gather implements pool.Pool. It blocks until the pool has enough transactions
// according to the configuration and then returns the transactions.
func (p *Pool) Gather(ctx context.Context, cfg pool.Config) []txn.Transaction {
	p.Lock()

	if len(p.bag) >= cfg.Min {
		txs := p.makeArray()
		p.Unlock()

		return txs
	}

	p.Unlock()

	return p.gatherer.Wait(ctx, cfg)
}

// Close stops the gossiper.
func (p *Pool) Close() error {
	close(p.closing)

	err := p.actor.Close()
	if err != nil {
		return xerrors.Errorf("failed to close gossiper: %v", err)
	}

	return nil
}

func (p *Pool) setTx(tx txn.Transaction) {
	p.Lock()

	p.bag[string(tx.GetID())] = tx

	p.gatherer.Notify(len(p.bag), p.makeArray)

	p.Unlock()
}

func (p *Pool) makeArray() []txn.Transaction {
	txs := make([]txn.Transaction, 0, len(p.bag))
	for _, tx := range p.bag {
		txs = append(txs, tx)
	}

	return txs
}

func (p *Pool) listenRumors(ch <-chan gossip.Rumor) {
	for {
		select {
		case r := <-ch:
			tx, ok := r.(txn.Transaction)
			if ok {
				p.setTx(tx)
			}
		case <-p.closing:
			return
		}
	}
}
