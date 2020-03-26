package byzcoin

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTxQueue_GetAll(t *testing.T) {
	q := newTxBag()

	q.buffer[Digest{1}] = Transaction{}
	q.buffer[Digest{2}] = Transaction{}

	txs := q.GetAll()
	require.Len(t, txs, 2)
}

func TestTxQueue_Add(t *testing.T) {
	q := newTxBag()

	q.Add(Transaction{hash: Digest{1}})
	q.Add(Transaction{hash: Digest{2}})
	require.Len(t, q.buffer, 2)

	q.Add(Transaction{hash: Digest{1}})
	require.Len(t, q.buffer, 2)
}

func TestTxQueue_Remove(t *testing.T) {
	q := newTxBag()

	q.buffer[Digest{1}] = Transaction{}
	q.buffer[Digest{2}] = Transaction{}
	q.buffer[Digest{3}] = Transaction{}
	q.Remove(TransactionResult{txID: Digest{2}})
	require.Len(t, q.buffer, 2)

	q.Remove(TransactionResult{txID: Digest{}})
	require.Len(t, q.buffer, 2)

	q.Remove(
		TransactionResult{txID: Digest{1}},
		TransactionResult{txID: Digest{2}},
		TransactionResult{txID: Digest{3}},
	)
	require.Empty(t, q.buffer)
}
