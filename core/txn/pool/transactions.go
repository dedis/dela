package pool

import (
	"bytes"
	"sort"

	"go.dedis.ch/dela/core/txn"
)

// Transactions is a sortable list of transactions.
//
// - implements sort.Interface
type transactions []transactionStats

// Len implements sort.Interface. It returns the length of the list.
func (txs transactions) Len() int {
	return len(txs)
}

// Less implements sort.Interface. It returns true if the nonce of the ith
// transaction is smaller than the jth.
func (txs transactions) Less(i, j int) bool {
	return txs[i].GetNonce() < txs[j].GetNonce()
}

// Swap implements sort.Interface. It swaps the ith and the jth transactions.
func (txs transactions) Swap(i, j int) {
	txs[i], txs[j] = txs[j], txs[i]
}

// Add adds the transaction to the list if and only if the nonce is unique. The
// resulting list will be sorted by nonce.
func (txs transactions) Add(other transactionStats) transactions {
	for _, tx := range txs {
		if tx.GetNonce() == other.GetNonce() {
			return txs
		}
	}

	list := append(txs, other)
	sort.Sort(list)

	return list
}

// Remove removes the transaction from the list if it exists, while preserving
// the order of the transactions.
func (txs transactions) Remove(other txn.Transaction) transactions {
	for i, tx := range txs {
		if bytes.Equal(tx.GetID(), other.GetID()) {
			txs = append(txs[:i], txs[i+1:]...)
			break
		}
	}

	return txs
}
