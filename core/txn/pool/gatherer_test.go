package pool

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/txn"
)

func TestSimpleGatherer_Wait(t *testing.T) {
	gatherer := NewSimpleGatherer().(*simpleGatherer)

	ctx := context.Background()

	cb := func() {
		gatherer.Lock()

		require.Len(t, gatherer.queue, 1)

		gatherer.queue[0].ch <- []txn.Transaction{}

		gatherer.Unlock()
	}

	txs := gatherer.Wait(ctx, Config{Callback: cb})
	require.NotNil(t, txs)
	require.Len(t, txs, 0)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	txs = gatherer.Wait(ctx, Config{})
	require.Nil(t, txs)
}

func TestSimpleGatherer_Notify(t *testing.T) {
	gatherer := NewSimpleGatherer().(*simpleGatherer)

	ch := make(chan []txn.Transaction, 1)
	gatherer.queue = []item{
		{cfg: Config{Min: 3}},
		{ch: ch, cfg: Config{Min: 2}},
		{cfg: Config{Min: 5}},
	}

	gatherer.Notify(2, func() []txn.Transaction { return nil })
	require.Len(t, gatherer.queue, 2)
	require.Len(t, <-ch, 0)
}
