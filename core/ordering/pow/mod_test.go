package pow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/execution/baremetal"
	"go.dedis.ch/dela/core/store"
	trie "go.dedis.ch/dela/core/store/mem"
	"go.dedis.ch/dela/core/tap"
	txn "go.dedis.ch/dela/core/tap/anon"
	pool "go.dedis.ch/dela/core/tap/pool/mem"
	validation "go.dedis.ch/dela/core/validation/simple"
)

func TestService_Basic(t *testing.T) {
	pool := pool.NewPool()
	srvc := NewService(pool, validation.NewService(baremetal.NewExecution(testExec{})), trie.NewTrie())

	// 1. Start the ordering service.
	require.NoError(t, srvc.Listen())
	defer srvc.Close()

	// 2. Watch for new events before sending a transaction.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	evts := srvc.Watch(ctx)

	// 3. Send a transaction to the pool. It should be detected by the ordering
	// service and start a new block.
	require.NoError(t, pool.Add(makeTx(t, 0)))

	evt := <-evts
	require.Equal(t, uint64(1), evt.Index)

	pr, err := srvc.GetProof([]byte("ping"))
	require.NoError(t, err)
	require.Equal(t, []byte("pong"), pr.GetValue())

	// 4. Send another transaction to the pool. This time it should creates a
	// block appended to the previous one.
	require.NoError(t, pool.Add(makeTx(t, 1)))

	evt = <-evts
	require.Equal(t, uint64(2), evt.Index)
}

// -----------------------------------------------------------------------------
// Utility functions

func makeTx(t *testing.T, nonce uint64) tap.Transaction {
	tx, err := txn.NewTransaction(nonce, txn.WithArg("key", []byte("ping")), txn.WithArg("value", []byte("pong")))
	require.NoError(t, err)

	return tx
}

type testExec struct{}

func (e testExec) Execute(tx tap.Transaction, store store.ReadWriteTrie) (execution.Result, error) {
	key := tx.GetArg("key")
	value := tx.GetArg("value")

	if len(key) == 0 || len(value) == 0 {
		return execution.Result{Accepted: false, Message: "key or value is nil"}, nil
	}

	err := store.Set(key, value)
	if err != nil {
		return execution.Result{}, err
	}

	return execution.Result{Accepted: true}, nil
}
