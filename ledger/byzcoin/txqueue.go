package byzcoin

import "sync"

type txQueue struct {
	sync.Mutex
	buffer map[Digest]Transaction
}

func newTxQueue() *txQueue {
	return &txQueue{
		buffer: make(map[Digest]Transaction),
	}
}

func (q *txQueue) GetAll() []Transaction {
	q.Lock()
	defer q.Unlock()

	txs := make([]Transaction, 0, len(q.buffer))
	for _, tx := range q.buffer {
		txs = append(txs, tx)
	}

	return txs
}

func (q *txQueue) Add(tx Transaction) {
	q.Lock()
	q.buffer[tx.hash] = tx
	q.Unlock()
}

func (q *txQueue) Remove(res ...TransactionResult) {
	q.Lock()
	for _, txResult := range res {
		delete(q.buffer, txResult.txID)
	}
	q.Unlock()
}
