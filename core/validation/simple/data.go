package simple

import (
	"io"

	"go.dedis.ch/dela/core/tap"
	"go.dedis.ch/dela/core/validation"
	"golang.org/x/xerrors"
)

// TransactionResult is the result of a transaction processing. It contains the
// transaction and its state of success.
//
// - implements validation.TransactionResult
type TransactionResult struct {
	tx       tap.Transaction
	accepted bool
	reason   string
}

// NewTransactionResult creates a new transaction result for the provided
// transaction.
func NewTransactionResult(tx tap.Transaction) TransactionResult {
	return TransactionResult{
		tx:       tx,
		accepted: true,
		reason:   "",
	}
}

// GetTransaction implements validation.TransactionResult. It returns the
// transaction associated to the result.
func (res TransactionResult) GetTransaction() tap.Transaction {
	return res.tx
}

// GetStatus implements validation.TransactionResult. It returns true if the
// transaction has been accepted, otherwise false with the reason.
func (res TransactionResult) GetStatus() (bool, string) {
	return res.accepted, res.reason
}

// Data is the validated data of a standard validation.
//
// - implements validation.Data
type Data struct {
	txs []TransactionResult
}

// GetTransactionResults implements validation.Data. It returns the results.
func (d Data) GetTransactionResults() []validation.TransactionResult {
	res := make([]validation.TransactionResult, len(d.txs))
	for i, r := range d.txs {
		res[i] = r
	}

	return res
}

// Fingerprint implements serde.Fingerprinter. It writes a deterministic binary
// representation of the validated data.
func (d Data) Fingerprint(w io.Writer) error {
	for _, res := range d.txs {
		err := res.tx.Fingerprint(w)
		if err != nil {
			return xerrors.Errorf("couldn't fingerprint tx: %v", err)
		}

		bit := []byte{0}
		if res.accepted {
			bit[0] = 1
		}

		_, err = w.Write(bit)
		if err != nil {
			return xerrors.Errorf("couldn't write accepted: %v", err)
		}
	}

	return nil
}
