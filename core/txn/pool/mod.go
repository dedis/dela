// Package pool defines the interface for a transaction pool. It will hold the
// transactions of the clients until an ordering service read them and it will
// broadcast the state of the pool to other known participants.
package pool

import (
	"context"

	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/mino"
)

// Config is the set of parameters that allows one to change the behavior of the
// gathering process.
type Config struct {
	// Min indicates what is minimum number of transactions that is required
	// before returning.
	Min int

	// Callback is a function called when the pool doesn't have enough
	// transactions at the moment of calling and must therefore wait for new
	// transactions to come. It allows one to take action to stop the gathering
	// if necessary.
	Callback func()
}

// Filter is the interface to implement to validate if a transaction will be
// accepted and thus is allowed to be pushed in the pool.
type Filter interface {
	// Accept returns an error when the transaction is going to be rejected.
	Accept(tx txn.Transaction, leeway validation.Leeway) error
}

// Pool is the maintainer of the list of transactions.
type Pool interface {
	// SetPlayers updates the list of participants that should eventually
	// receive the transactions.
	SetPlayers(mino.Players) error

	AddFilter(Filter)

	// Len returns the number of transactions available in the pool.
	Len() int

	// Add adds the transaction to the pool.
	Add(txn.Transaction) error

	// Remove removes the transaction from the pool.
	Remove(txn.Transaction) error

	// Gather is a blocking function to gather transactions from the pool. The
	// configuration allows one to specify criterion before returning.
	Gather(context.Context, Config) []txn.Transaction

	// Close closes the pool and cleans the resources.
	Close() error
}
