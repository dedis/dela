package mem

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/pool"
)

func TestPool_Add(t *testing.T) {
	pool := NewPool()

	err := pool.Add(fakeTx{id: []byte{1}})
	require.NoError(t, err)

	err = pool.Add(fakeTx{id: []byte{2}})
	require.NoError(t, err)

	// A transaction that exists in the active queue can simply be overwritten
	// thus no error is expected.
	err = pool.Add(fakeTx{id: []byte{1}})
	require.NoError(t, err)

	longID := make([]byte, 33)
	err = pool.Add(fakeTx{id: longID})
	require.EqualError(t, err, "tx identifier is too long: 33 > 32")

	pool.history[Key{3}] = struct{}{}
	err = pool.Add(fakeTx{id: []byte{3}})
	require.EqualError(t, err, fmt.Sprintf("tx %#x already exists", Key{3}))
}

func TestPool_Remove(t *testing.T) {
	pool := NewPool()

	pool.txs[Key{1}] = fakeTx{}

	err := pool.Remove(fakeTx{id: []byte{1}})
	require.NoError(t, err)
	require.Len(t, pool.history, 1)

	err = pool.Remove(fakeTx{id: []byte{1}})
	require.EqualError(t, err, fmt.Sprintf("transaction %#x not found", Key{1}))
}

func TestPool_SetPlayers(t *testing.T) {
	pool := NewPool()

	require.NoError(t, pool.SetPlayers(nil))
}

func TestPool_Gather(t *testing.T) {
	p := NewPool()

	ctx := context.Background()

	cb := func() {
		require.NoError(t, p.Add(fakeTx{}))
	}

	txs := p.Gather(ctx, pool.Config{Min: 1, Callback: cb})
	require.Len(t, txs, 1)

	txs = p.Gather(ctx, pool.Config{Min: 1})
	require.Len(t, txs, 1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	txs = p.Gather(ctx, pool.Config{Min: 2})
	require.Len(t, txs, 0)
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeTx struct {
	txn.Transaction

	id []byte
}

func (tx fakeTx) GetID() []byte {
	return tx.id
}
