package execution

import (
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/txn"
)

// Result is the result of a transaction execution.
type Result struct {
	// Accepted is the success state of the transaction.
	Accepted bool

	// Message gives a change to the execution to explain why a transaction has
	// failed.
	Message string
}

// Service is the execution service that defines the primitives to execute a
// transaction.
type Service interface {
	// Execute must apply the transaction to the trie and return the result of
	// it.
	Execute(tx txn.Transaction, snap store.Snapshot) (Result, error)
}
