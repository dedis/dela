package byzcoin

import "sync"

// txQueue is a storage abstraction where the transactions are stored while
// waiting to be included in a block.
type txQueue struct {
	sync.Mutex
	buffer map[Digest]Transaction
}

func newTxQueue() *txQueue {
	return &txQueue{
		buffer: make(map[Digest]Transaction),
	}
}

// GetAll returns a list of the transactions currently queued.
func (q *txQueue) GetAll() []Transaction {
	q.Lock()
	defer q.Unlock()

	txs := make([]Transaction, 0, len(q.buffer))
	for _, tx := range q.buffer {
		txs = append(txs, tx)
	}

	return txs
}

// Add adds the transaction to the queue.
func (q *txQueue) Add(tx Transaction) {
	q.Lock()
	q.buffer[tx.hash] = tx
	q.Unlock()
}

// Remove deletes the transactions associated with the transaction results.
func (q *txQueue) Remove(res ...TransactionResult) {
	q.Lock()
	for _, txResult := range res {
		delete(q.buffer, txResult.txID)
	}
	q.Unlock()
}
