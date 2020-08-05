// Package gossip implements a transaction pool that is using a gossip protocol
// to spread the transactions to other participants.
package gossip

import (
	"context"
	"sync"

	"go.dedis.ch/dela/blockchain"
	"go.dedis.ch/dela/core/tap"
	"go.dedis.ch/dela/core/tap/pool"
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
	actor   gossip.Actor
	bag     map[string]tap.Transaction
	watcher blockchain.Observable
	closing chan struct{}
}

// NewPool creates a new empty pool and starts to gossip incoming transaction.
func NewPool(gossiper gossip.Gossiper) (*Pool, error) {
	actor, err := gossiper.Listen()
	if err != nil {
		return nil, xerrors.Errorf("failed to listen: %v", err)
	}

	p := &Pool{
		actor:   actor,
		bag:     make(map[string]tap.Transaction),
		watcher: blockchain.NewWatcher(),
		closing: make(chan struct{}),
	}

	go p.listenRumors(gossiper.Rumors())

	return p, nil
}

// Len implements pool.Pool. It returns the current length of the pool.
func (p *Pool) Len() int {
	p.Lock()
	defer p.Unlock()

	return len(p.bag)
}

// GetAll implements pool.Pool. It returns the list of transactions that have
// been received. It does not remove the transactions from the pool, thus
// consecutive calls can return the same list.
func (p *Pool) GetAll() []tap.Transaction {
	p.Lock()
	defer p.Unlock()

	txs := make([]tap.Transaction, 0, len(p.bag))
	for _, tx := range p.bag {
		txs = append(txs, tx)
	}

	return txs
}

// SetPlayers implements pool.Pool. It sets the list of participants the
// transactions should be gossiped to.
func (p *Pool) SetPlayers(players mino.Players) error {
	p.actor.SetPlayers(players)
	return nil
}

// Add implements pool.Pool. It adds the transaction to the pool and gossips it
// to other participants.
func (p *Pool) Add(tx tap.Transaction) error {
	err := p.actor.Add(tx)
	if err != nil {
		return xerrors.Errorf("failed to gossip tx: %v", err)
	}

	p.setTx(tx)
	p.notify()

	return nil
}

// Remove implements pool.Pool. It removes the transaction from the pool.
func (p *Pool) Remove(tx tap.Transaction) error {
	p.Lock()
	delete(p.bag, string(tx.GetID()))
	p.Unlock()

	p.notify()

	return nil
}

// Watch implements pool.Pool. It returns a channel that will be populated by
// events when the pool changes.
func (p *Pool) Watch(ctx context.Context) <-chan pool.Event {
	ch := make(chan pool.Event, 1)
	obs := observer{ch: ch}

	p.watcher.Add(obs)

	go func() {
		<-ctx.Done()
		p.watcher.Remove(obs)
	}()

	return ch
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

func (p *Pool) setTx(tx tap.Transaction) {
	p.Lock()
	p.bag[string(tx.GetID())] = tx
	p.Unlock()
}

func (p *Pool) notify() {
	// Notify is done independently and the lock is only acquired to create the
	// event but released so that spreading the event doesn't slow down other
	// functions.
	p.Lock()
	event := pool.Event{
		Len: len(p.bag),
	}
	p.Unlock()

	p.watcher.Notify(event)
}

func (p *Pool) listenRumors(ch <-chan gossip.Rumor) {
	for {
		select {
		case r := <-ch:
			tx, ok := r.(tap.Transaction)
			if ok {
				p.setTx(tx)
				p.notify()
			}
		case <-p.closing:
			return
		}
	}
}

type observer struct {
	ch chan pool.Event
}

func (obs observer) NotifyCallback(event interface{}) {
	obs.ch <- event.(pool.Event)
}
