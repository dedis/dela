package byzcoin

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/ledger"
)

func TestTxQueue_GetAll(t *testing.T) {
	q := newTxBag()

	q.buffer[Key{1}] = fakeTx{}
	q.buffer[Key{2}] = fakeTx{}

	txs := q.GetAll()
	require.Len(t, txs, 2)
}

func TestTxQueue_Add(t *testing.T) {
	q := newTxBag()

	q.Add(fakeTx{id: []byte{1}})
	q.Add(fakeTx{id: []byte{2}})
	require.Len(t, q.buffer, 2)

	q.Add(fakeTx{id: []byte{1}})
	require.Len(t, q.buffer, 2)
}

func TestTxQueue_Remove(t *testing.T) {
	q := newTxBag()

	q.buffer[Key{1}] = fakeTx{}
	q.buffer[Key{2}] = fakeTx{}
	q.buffer[Key{3}] = fakeTx{}
	q.Remove(TransactionResult{txID: []byte{2}})
	require.Len(t, q.buffer, 2)

	q.Remove(TransactionResult{txID: []byte{}})
	require.Len(t, q.buffer, 2)

	q.Remove(
		TransactionResult{txID: []byte{1}},
		TransactionResult{txID: []byte{2}},
		TransactionResult{txID: []byte{3}},
	)
	require.Empty(t, q.buffer)
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeTx struct {
	ledger.Transaction
	id []byte
}

func (tx fakeTx) GetID() []byte {
	return tx.id
}
