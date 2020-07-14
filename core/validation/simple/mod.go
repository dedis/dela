package simple

import (
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/tap"
	"go.dedis.ch/dela/core/validation"
	"golang.org/x/xerrors"
)

// Service is a standard validation service that will process the batch and
// update a trie accordingly. This trie will have a root to identify the
// validated data.
type Service struct {
	execution execution.Service
}

// NewService creates a new validation service.
func NewService(exec execution.Service) Service {
	return Service{
		execution: exec,
	}
}

// Validate implements validation.Service. It validates the list of transactions
// and return the validated data.
func (s Service) Validate(trie store.ReadWriteTrie, txs []tap.Transaction) (validation.Data, error) {
	results := make([]TransactionResult, len(txs))

	for i, tx := range txs {
		res, err := s.execution.Execute(tx, trie)
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
