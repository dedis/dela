// Package validation defines a validation service that will apply a batch of
// transactions to a store snapshot.
package validation

import (
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/tap"
	"go.dedis.ch/dela/serde"
)

// TransactionResult is the result of a transaction execution.
type TransactionResult interface {
	serde.Message

	// GetTransaction returns the transaction associated to the result.
	GetTransaction() tap.Transaction

	// GetStatus returns the status of the execution. It returns true if the
	// transaction has been accepted, otherwise false with a message to explain
	// the reason.
	GetStatus() (bool, string)
}

// Data is the result of a validation.
type Data interface {
	serde.Message
	serde.Fingerprinter

	// GetTransactionResults returns the results.
	GetTransactionResults() []TransactionResult
}

type DataFactory interface {
	serde.Factory

	DataOf(serde.Context, []byte) (Data, error)
}

// Service is the validation service that will process a batch of transactions
// into a validated data that can be used as a payload of a block.
type Service interface {
	GetFactory() DataFactory

	// Validate takes a snapshot and a list of transactions and returns a
	// validated data bundle.
	Validate(store.Snapshot, []tap.Transaction) (Data, error)
}
