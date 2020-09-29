package pool

import (
	"context"
	"fmt"
	"sync"

	"go.dedis.ch/dela/core/txn"
	"golang.org/x/xerrors"
)

// KeyMaxLength is the maximum length of a transaction identifier.
const KeyMaxLength = 32

// Key is the key of a transaction. By definition, it expects that the
// identifier of the transaction is no more than 32 bytes long.
type Key [KeyMaxLength]byte

// String implements fmt.Stringer. It returns a short string representation of
// the key.
func (k Key) String() string {
	return fmt.Sprintf("%#x", k[:4])
}

// Gatherer is a common tool to the pool implementations that helps to implement
// the gathering process.
type Gatherer interface {
	// Len returns the number of pending transactions.
	Len() int

	// Add adds the transaction to the list of pending transactions.
	Add(tx txn.Transaction) error

	// Remove removes a transaction from the list of pending ones.
	Remove(tx txn.Transaction) error

	// Wait waits for a notification with sufficient transactions to return the
	// array, or nil if the context ends.
	Wait(ctx context.Context, cfg Config) []txn.Transaction

	// Close closes current operations and cleans the resources.
	Close()
}

type item struct {
	cfg Config
	ch  chan []txn.Transaction
}

type simpleGatherer struct {
	sync.Mutex
	queue   []item
	set     map[Key]txn.Transaction
	history map[Key]struct{}
}

// NewSimpleGatherer creates a new gatherer.
func NewSimpleGatherer() Gatherer {
	return &simpleGatherer{
		set:     make(map[Key]txn.Transaction),
		history: make(map[Key]struct{}),
	}
}

// Len implements pool.Gatherer. It returns the number of transaction available
// in the pool.
func (g *simpleGatherer) Len() int {
	g.Lock()
	defer g.Unlock()

	return len(g.set)
}

// Add implements pool.Gatherer. It adds the transaction to the set of available
// transactions and notify the queue of the new length.
func (g *simpleGatherer) Add(tx txn.Transaction) error {
	id := tx.GetID()
	if len(id) > KeyMaxLength {
		return xerrors.Errorf("tx identifier is too long: %d > %d", len(id), KeyMaxLength)
	}

	key := Key{}
	copy(key[:], id)

	g.Lock()

	_, found := g.history[key]
	if found {
		g.Unlock()
		return xerrors.Errorf("tx %v already exists", key)
	}

	g.set[key] = tx

	g.notify(len(g.set))

	g.Unlock()

	return nil
}

// Remove implements pool.Gatherer. It removes the transaction from the set of
// available transactions and add the key in the history to prevent duplicates.
func (g *simpleGatherer) Remove(tx txn.Transaction) error {
	key := Key{}
	copy(key[:], tx.GetID())

	g.Lock()

	_, found := g.set[key]
	if !found {
		g.Unlock()
		return xerrors.Errorf("transaction %v not found", key)
	}

	delete(g.set, key)

	// Keep an history of transactions to prevent duplicates to be indefinitely
	// added to the pool.
	g.history[key] = struct{}{}

	g.Unlock()

	return nil
}

// Wait implements pool.Gatherer. It waits for enough transactions before
// returning the list, or it returns nil if the context ends.
func (g *simpleGatherer) Wait(ctx context.Context, cfg Config) []txn.Transaction {
	ch := make(chan []txn.Transaction, 1)

	g.Lock()

	if len(g.set) >= cfg.Min {
		txs := g.makeArray()
		g.Unlock()

		return txs
	}

	g.queue = append(g.queue, item{cfg: cfg, ch: ch})

	g.Unlock()

	if cfg.Callback != nil {
		cfg.Callback()
	}

	select {
	case txs := <-ch:
		return txs
	case <-ctx.Done():
		return nil
	}
}

// Close implements pool.Gatherer. It closes the operations and cleans the
// resources.
func (g *simpleGatherer) Close() {
	g.Lock()

	g.history = make(map[Key]struct{})
	g.set = make(map[Key]txn.Transaction)

	for _, item := range g.queue {
		close(item.ch)
	}

	g.queue = nil

	g.Unlock()
}

// Notify implements pool.Gatherer. It triggers the elements of the queue that
// are waiting for at least the length in parameter and remove them from the
// queue.
func (g *simpleGatherer) notify(length int) {
	// Iterating by descending order to allow the deletion of the element inside
	// the loop.
	for i := len(g.queue) - 1; i >= 0; i-- {
		item := g.queue[i]

		if item.cfg.Min <= length {
			item.ch <- g.makeArray()
			g.queue = append(g.queue[:i], g.queue[i+1:]...)
		}
	}
}

func (g *simpleGatherer) makeArray() []txn.Transaction {
	txs := make([]txn.Transaction, 0, len(g.set))
	for _, tx := range g.set {
		txs = append(txs, tx)
	}

	return txs
}
