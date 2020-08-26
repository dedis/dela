package pool

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/txn"
)

func TestSimpleGatherer_Add(t *testing.T) {
	gatherer := NewSimpleGatherer().(*simpleGatherer)

	err := gatherer.Add(fakeTx{})
	require.NoError(t, err)

	err = gatherer.Add(fakeTx{id: make([]byte, 33)})
	require.EqualError(t, err, "tx identifier is too long: 33 > 32")

	gatherer.history[Key{}] = struct{}{}
	err = gatherer.Add(fakeTx{})
	require.EqualError(t, err, "tx 0x00000000 already exists")
}

func TestSimpleGatherer_Remove(t *testing.T) {
	gatherer := NewSimpleGatherer().(*simpleGatherer)
	gatherer.set[Key{}] = fakeTx{}

	err := gatherer.Remove(fakeTx{})
	require.NoError(t, err)

	err = gatherer.Remove(fakeTx{})
	require.EqualError(t, err, "transaction 0x00000000 not found")
}

func TestSimpleGatherer_Wait(t *testing.T) {
	gatherer := NewSimpleGatherer().(*simpleGatherer)

	ctx := context.Background()

	cb := func() {
		gatherer.Lock()
		require.Len(t, gatherer.queue, 1)
		gatherer.Unlock()

		require.NoError(t, gatherer.Add(fakeTx{}))
	}

	txs := gatherer.Wait(ctx, Config{Min: 1, Callback: cb})
	require.Len(t, txs, 1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	txs = gatherer.Wait(ctx, Config{Min: 1})
	require.Len(t, txs, 1)

	txs = gatherer.Wait(ctx, Config{Min: 2})
	require.Nil(t, txs)
}

// Utility functions -----------------------------------------------------------

type fakeTx struct {
	txn.Transaction

	id []byte
}

func (tx fakeTx) GetID() []byte {
	return tx.id
}
