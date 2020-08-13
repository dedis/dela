package pool

import (
	"context"
	"sync"

	"go.dedis.ch/dela/core/txn"
)

// Gatherer is a common tool to the pool implementations that helps to implement
// the gathering process.
type Gatherer interface {
	// Wait waits for a notification with sufficient transactions to return the
	// array, or nil if the context ends.
	Wait(ctx context.Context, cfg Config) []txn.Transaction

	// Notify triggers any wait that fits the given lenght. The function is only
	// called when necessary.
	Notify(length int, fn func() []txn.Transaction)
}

type item struct {
	cfg Config
	ch  chan []txn.Transaction
}

type simpleGatherer struct {
	sync.Mutex
	queue []item
}

// NewSimpleGatherer creates a new gatherer.
func NewSimpleGatherer() Gatherer {
	return &simpleGatherer{}
}

// Wait implements pool.Gatherer. It waits for enough transactions before
// returning the list, or it returns nil if the context ends.
func (g *simpleGatherer) Wait(ctx context.Context, cfg Config) []txn.Transaction {
	ch := make(chan []txn.Transaction, 1)

	g.Lock()
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

// Notify implements pool.Gatherer. It triggers the elements of the queue that
// are waiting for at least the length in parameter and remove them from the
// queue.
func (g *simpleGatherer) Notify(length int, fn func() []txn.Transaction) {
	g.Lock()

	// Iterating by descending order to allow the deletion of the element inside
	// the loop.
	for i := len(g.queue) - 1; i >= 0; i-- {
		item := g.queue[i]

		if item.cfg.Min <= length {
			item.ch <- fn()
			g.queue = append(g.queue[:i], g.queue[i+1:]...)
		}
	}

	g.Unlock()
}
