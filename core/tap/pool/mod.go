// Package pool defines the interface for a transaction pool. It will hold the
// transactions of the clients until an ordering service read them and it will
// broadcast the state of the pool to other known participants.
package pool

import (
	"context"

	"go.dedis.ch/dela/core/tap"
)

// Event is an event triggered when new transactions arrived to the pool.
type Event struct {
	// Len is the current length of the pool.
	Len int
}

// Pool is the maintainer of the list of transactions.
type Pool interface {
	Len() int

	Add(tap.Transaction) error

	GetAll() []tap.Transaction

	Watch(context.Context) <-chan Event
}
