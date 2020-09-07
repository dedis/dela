// Package simple implements a simple validation service.
package simple

import (
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/validation"
	"golang.org/x/xerrors"
)

// Service is a standard validation service that will process the batch and
// update the snapshot accordingly.
//
// - implements validation.Service
type Service struct {
	execution execution.Service
	fac       validation.DataFactory
}

// NewService creates a new validation service.
func NewService(exec execution.Service, f txn.Factory) Service {
	return Service{
		execution: exec,
		fac:       NewDataFactory(f),
	}
}

// GetFactory implements validation.Service. It returns the factory for the
// validated data.
func (s Service) GetFactory() validation.DataFactory {
	return s.fac
}

// Validate implements validation.Service. It processes the list of transactions
// while updating the snapshot then returns a bundle of the transaction results.
func (s Service) Validate(store store.Snapshot, txs []txn.Transaction) (validation.Data, error) {
	results := make([]TransactionResult, len(txs))

	for i, tx := range txs {
		res, err := s.execution.Execute(tx, store)
		if err != nil {
			// This is a critical error unrelated to the transaction itself.
			return nil, xerrors.Errorf("failed to execute tx: %v", err)
		}

		results[i] = TransactionResult{
			tx:       tx,
			accepted: res.Accepted,
		}
	}

	data := Data{
		txs: results,
	}

	return data, nil
}
