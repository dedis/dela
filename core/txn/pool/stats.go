package pool

import (
	"time"

	"go.dedis.ch/dela/core/txn"
)

// transactionStats enhances a transaction with some statistics
// to allow the detection of rotten transactions in a pool.
type transactionStats struct {
	txn.Transaction
	insertionTime time.Time
}

// ResetStats resets the insertion time to now.
// It is used when a leader view change is initiated.
func (t *transactionStats) ResetStats() {
	t.insertionTime = time.Now()
}
