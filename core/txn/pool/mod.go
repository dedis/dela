// Package pool defines the interface for a transaction pool. It will hold the
// transactions of the clients until an ordering service read them and it will
// broadcast the state of the pool to other known participants.
package pool

import (
	"context"

	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/mino"
)

// Event is an event triggered when new transactions arrived to the pool.
type Event struct {
	// Len is the current length of the pool.
	Len int
}

// Pool is the maintainer of the list of transactions.
type Pool interface {
	// Len returns the length of the pool.
	Len() int

	// GetAll returns the list of transactions available.
	GetAll() []txn.Transaction

	// SetPlayers updates the list of participants that should eventually
	// receive the transactions.
	SetPlayers(mino.Players) error

	// Add adds the transaction to the pool.
	Add(txn.Transaction) error

	// Remove removes the transaction from the pool.
	Remove(txn.Transaction) error

	// Watch returns a channel of events that will be populated when the length
	// of the pool evolves.
	Watch(context.Context) <-chan Event
}
