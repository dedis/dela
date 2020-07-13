// Package validation defines the validator of a block created by an ordering
// service.
package validation

import (
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/tap"
	"go.dedis.ch/dela/serde"
)

type TransactionResult interface {
	GetTransaction() tap.Transaction

	GetStatus() (bool, string)
}

// Data is the result of a validation.
type Data interface {
	serde.Fingerprinter

	GetTransactionResults() []TransactionResult
}

// Service is the validation service that will process a batch of transactions
// into a validated data that can be used as a payload of a block.
type Service interface {
	Validate(store.ReadWriteTrie, []tap.Transaction) (Data, error)
}
