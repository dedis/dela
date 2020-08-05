package mem

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/tap"
)

func TestPool_Len(t *testing.T) {
	pool := NewPool()

	require.Equal(t, 0, pool.Len())

	pool.txs[Key{}] = fakeTx{}
	require.Equal(t, 1, pool.Len())

	pool.txs[Key{1}] = fakeTx{}
	require.Equal(t, 2, pool.Len())
}

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

func TestPool_GetAll(t *testing.T) {
	pool := NewPool()

	pool.txs[Key{1}] = fakeTx{}
	pool.txs[Key{2}] = fakeTx{}

	txs := pool.GetAll()
	require.Len(t, txs, 2)
}

func TestPool_SetPlayers(t *testing.T) {
	pool := NewPool()

	require.NoError(t, pool.SetPlayers(nil))
}

func TestPool_Watch(t *testing.T) {
	pool := NewPool()
	pool.txs[Key{}] = fakeTx{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := pool.Watch(ctx)

	require.NoError(t, pool.Add(fakeTx{id: []byte{1}}))
	evt := <-events
	require.Equal(t, 2, evt.Len)

	cancel()

	select {
	case <-time.After(5 * time.Second):
		t.Fatal("expect events channel to close")
	case _, more := <-events:
		require.False(t, more)
	}
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeTx struct {
	tap.Transaction

	id []byte
}

func (tx fakeTx) GetID() []byte {
	return tx.id
}
