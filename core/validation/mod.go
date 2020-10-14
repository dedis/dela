// Package validation defines a validation service that will apply a batch of
// transactions to a store snapshot.
//
// Documentation Last Review: 08.10.2020
//
package validation

import (
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/serde"
)

// TransactionResult is the result of a transaction execution.
type TransactionResult interface {
	serde.Message

	// GetTransaction returns the transaction associated to the result.
	GetTransaction() txn.Transaction

	// GetStatus returns the status of the execution. It returns true if the
	// transaction has been accepted, otherwise false with a message to explain
	// the reason.
	GetStatus() (bool, string)
}

// Result is the result of a validation.
type Result interface {
	serde.Message
	serde.Fingerprinter

	// GetTransactionResults returns the results.
	GetTransactionResults() []TransactionResult
}

// ResultFactory is the factory for results.
type ResultFactory interface {
	serde.Factory

	ResultOf(serde.Context, []byte) (Result, error)
}

// Leeway is the configuration when asserting if a transaction will be accepted
// to lighten some of the constraints.
type Leeway struct {
	// MaxSequenceDifference defines how much from the current sequence the
	// transaction can differ.
	MaxSequenceDifference int
}

// Service is the validation service that will process a batch of transactions
// into a result that can be used as a payload of a block.
type Service interface {
	// GetFactory returns the result factory.
	GetFactory() ResultFactory

	// GetNonce returns the nonce associated with the identity. The value
	// returned should be used for the next transaction to be valid.
	GetNonce(store.Readable, access.Identity) (uint64, error)

	// Accept returns nil if the transaction will be accepted by the service.
	// The leeway parameter allows to reduce some constraints.
	Accept(store.Readable, txn.Transaction, Leeway) error

	// Validate takes a snapshot and a list of transactions and returns a
	// result.
	Validate(store.Snapshot, []txn.Transaction) (Result, error)
}
