package pool

import (
	"bytes"
	"context"
	"sort"
	"sync"

	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/validation"
	"golang.org/x/xerrors"
)

// DefaultIdentitySize is the default size defined for each identity to store
// transactions.
const DefaultIdentitySize = 10

// Transactions is a sortable list of transactions.
//
// - implements sort.Interface
type transactions []txn.Transaction

// Len implements sort.Interface. It returns the length of the list.
func (txs transactions) Len() int {
	return len(txs)
}

// Less implements sort.Interface. It returns true if the nonce of the ith
// transaction is smaller than the jth.
func (txs transactions) Less(i, j int) bool {
	return txs[i].GetNonce() < txs[j].GetNonce()
}

// Swap implements sort.Interface. It swaps the ith and the jth transactions.
func (txs transactions) Swap(i, j int) {
	txs[i], txs[j] = txs[j], txs[i]
}

// Add adds the transaction to the list if and only if the nonce is unique. The
// resulting list will be sorted by nonce.
func (txs transactions) Add(other txn.Transaction) transactions {
	for _, tx := range txs {
		if tx.GetNonce() == other.GetNonce() {
			return txs
		}
	}

	list := append(txs, other)
	sort.Sort(list)

	return list
}

// Remove removes the transaction from the list if it exists, while preserving
// the order of the transactions.
func (txs transactions) Remove(other txn.Transaction) transactions {
	for i, tx := range txs {
		if bytes.Equal(tx.GetID(), other.GetID()) {
			return append(txs[:i], txs[i+1:]...)
		}
	}

	return txs
}

// Gatherer is a common tool to the pool implementations that helps to implement
// the gathering process.
type Gatherer interface {
	// Len returns the number of pending transactions.
	Len() int

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
	// independently from each other.
	txs map[string]transactions
}

// NewSimpleGatherer creates a new gatherer.
func NewSimpleGatherer() Gatherer {
	return &simpleGatherer{
		limit: DefaultIdentitySize,
		txs:   make(map[string]transactions),
	}
}

// Len implements pool.Gatherer. It returns the number of transaction available
// in the pool.
func (g *simpleGatherer) Len() int {
	g.Lock()
	defer g.Unlock()

	return g.calculateLength()
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

	g.txs[key] = g.txs[key].Add(tx)

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

func (g *simpleGatherer) makeArray() []txn.Transaction {
	txs := make([]txn.Transaction, 0, g.calculateLength())
	for _, list := range g.txs {
		txs = append(txs, list...)
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
