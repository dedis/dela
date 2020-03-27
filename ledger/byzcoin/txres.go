package byzcoin

import (
	fmt "fmt"
)

// TransactionResult contains the result of a transaction execution.
//
// - implements ledger.TransactionResult
// - implements fmt.Stringer
type TransactionResult struct {
	txID     []byte
	Accepted bool
}

// GetTransactionID implements ledger.TransactionResult. It returns the unique
// identifier of the related transaction.
func (tr TransactionResult) GetTransactionID() []byte {
	return tr.txID
}

// String implements fmt.Stringer. It returns a string representation of the
// transaction result.
func (tr TransactionResult) String() string {
	return fmt.Sprintf("TransactionResult@%#x", tr.txID)
}
