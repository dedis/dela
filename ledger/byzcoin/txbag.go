package byzcoin

import (
	"sync"

	"go.dedis.ch/fabric/ledger/transactions"
)

// Key is type used to differentiate the transactions in the bag.
type Key [32]byte

// txBag is a storage abstraction where the transactions are stored while
// waiting to be included in a block.
type txBag struct {
	sync.Mutex
	buffer map[Key]transactions.ClientTransaction
}

func newTxBag() *txBag {
	return &txBag{
		buffer: make(map[Key]transactions.ClientTransaction),
	}
}

// GetAll returns a list of the transactions currently queued.
func (q *txBag) GetAll() []transactions.ClientTransaction {
	q.Lock()
	defer q.Unlock()

	txs := make([]transactions.ClientTransaction, 0, len(q.buffer))
	for _, tx := range q.buffer {
		txs = append(txs, tx)
	}

	return txs
}

// Add adds the transaction to the queue.
func (q *txBag) Add(tx transactions.ClientTransaction) {
	key := Key{}
	copy(key[:], tx.GetID())

	q.Lock()
	q.buffer[key] = tx
	q.Unlock()
}

// Remove deletes the transactions associated with the transaction results.
func (q *txBag) Remove(res ...TransactionResult) {
	q.Lock()
	for _, txResult := range res {
		key := Key{}
		copy(key[:], txResult.txID)
		delete(q.buffer, key)
	}
	q.Unlock()
}
