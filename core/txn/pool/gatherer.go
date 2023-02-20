package pool

import (
	"context"
	"sync"
	"time"

	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/validation"
	"golang.org/x/xerrors"
)

// DefaultIdentitySize is the default size defined for each identity to store
// transactions.
const DefaultIdentitySize = 100

// Gatherer is a common tool to the pool implementations that helps to implement
// the gathering process.
type Gatherer interface {
	// AddFilter adds the filter to the list that a transaction will go through
	// before being accepted by the gatherer.
	AddFilter(Filter)

	// Add adds the transaction to the list of pending transactions.
	Add(tx txn.Transaction) error

	// Remove removes a transaction from the list of pending ones.
	Remove(tx txn.Transaction) error

	// Wait waits for a notification with sufficient transactions to return the
	// array, or nil if the context ends.
	Wait(ctx context.Context, cfg Config) []txn.Transaction

	// Close closes current operations and cleans the resources.
	Close()

	// Stats gets the transaction statistics.
	Stats() Stats

	// ResetStats resets the transaction statistics.
	ResetStats()
}

type item struct {
	cfg Config
	ch  chan []txn.Transaction
}

// SimpleGatherer is a gatherer of transactions that will use filters to drop
// invalid transactions. It limits the size for each identity, *as long as* a
// filter is set, otherwise it can grow indefinitely.
//
// - implements pool.Gatherer
type simpleGatherer struct {
	sync.Mutex

	limit      int
	queue      []item
	validators []Filter

	// A string key is generated for each unique identity, which will have its
	// own list of transactions, so that a limited size can be enforced
	// independently of each other.
	txs map[string]transactions
}

// NewSimpleGatherer creates a new gatherer.
func NewSimpleGatherer() Gatherer {
	return &simpleGatherer{
		limit: DefaultIdentitySize,
		txs:   make(map[string]transactions),
	}
}

// AddFilter implements pool.Gatherer. It adds the filter to the list that a
// transaction will go through before being accepted by the gatherer.
func (g *simpleGatherer) AddFilter(filter Filter) {
	if filter == nil {
		return
	}

	g.validators = append(g.validators, filter)
}

// Add implements pool.Gatherer. It adds the transaction to the set of available
// transactions and notify the queue of the new length.
func (g *simpleGatherer) Add(tx txn.Transaction) error {

	for _, val := range g.validators {
		// Make sure the transaction is not already known, or that is not in a
		// distant future to limit the pool storage size.
		err := val.Accept(tx, validation.Leeway{MaxSequenceDifference: g.limit})
		if err != nil {
			return xerrors.Errorf("invalid transaction: %v", err)
		}
	}

	key, err := makeKey(tx.GetIdentity())
	if err != nil {
		return xerrors.Errorf("identity key failed: %v", err)
	}

	g.Lock()

	g.txs[key] = g.txs[key].Add(transactionStats{
		tx,
		time.Now(),
	})

	g.notify(g.calculateLength())

	g.Unlock()

	return nil
}

// Remove implements pool.Gatherer. It removes the transaction from the set of
// available transactions and add the key in the history to prevent duplicates.
func (g *simpleGatherer) Remove(tx txn.Transaction) error {
	key, err := makeKey(tx.GetIdentity())
	if err != nil {
		return xerrors.Errorf("identity key failed: %v", err)
	}

	g.Lock()

	g.txs[key] = g.txs[key].Remove(tx)

	g.Unlock()

	return nil
}

// Wait implements pool.Gatherer. It waits for enough transactions before
// returning the list, or it returns nil if the context ends.
func (g *simpleGatherer) Wait(ctx context.Context, cfg Config) []txn.Transaction {
	ch := make(chan []txn.Transaction, 1)

	g.Lock()

	if g.calculateLength() >= cfg.Min {
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

// Stats implements pool.Gatherer. It gets the transaction statistics.
func (g *simpleGatherer) Stats() Stats {
	g.Lock()
	defer g.Unlock()

	txs := g.makeStatsArray()
	stats := Stats{
		TxCount:  len(txs),
		OldestTx: time.Now(),
	}

	for _, tx := range txs {
		if tx.insertionTime.Before(stats.OldestTx) {
			stats.OldestTx = tx.insertionTime
		}
	}

	return stats
}

// ResetStats implements pool.Gatherer. It resets the transactions statistics.
func (g *simpleGatherer) ResetStats() {
	g.Lock()
	defer g.Unlock()

	txs := g.makeStatsArray()
	for _, tx := range txs {
		tx.ResetStats()
	}
}

// Close implements pool.Gatherer. It closes the operations and cleans the
// resources.
func (g *simpleGatherer) Close() {
	g.Lock()

	g.txs = make(map[string]transactions)

	for _, item := range g.queue {
		close(item.ch)
	}

	g.queue = nil

	g.Unlock()
}

// Notify triggers the elements of the queue that are waiting for at least the
// length in parameter and remove them from the queue.
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

func (g *simpleGatherer) calculateLength() int {
	num := 0
	for _, list := range g.txs {
		num += len(list)
	}

	return num
}

func (g *simpleGatherer) makeStatsArray() []transactionStats {
	txs := make([]transactionStats, 0, g.calculateLength())
	for _, list := range g.txs {
		txs = append(txs, list...)
	}

	return txs
}

func (g *simpleGatherer) makeArray() []txn.Transaction {
	stxs := g.makeStatsArray()
	txs := make([]txn.Transaction, 0, len(stxs))

	for _, t := range stxs {
		txs = append(txs, t.Transaction)
	}

	return txs
}

func makeKey(id access.Identity) (string, error) {
	data, err := id.MarshalText()
	if err != nil {
		return "", err
	}

	return string(data), nil
}
