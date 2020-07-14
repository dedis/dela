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
	"go.dedis.ch/dela/core/validation"
	val "go.dedis.ch/dela/core/validation/simple"
	"golang.org/x/xerrors"
)

func TestService_Basic(t *testing.T) {
	pool := pool.NewPool()
	srvc := NewService(pool, val.NewService(baremetal.NewExecution(testExec{})), trie.NewTrie())

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

func TestService_Listen(t *testing.T) {
	pool := pool.NewPool()
	srvc := NewService(pool, val.NewService(baremetal.NewExecution(testExec{})), trie.NewTrie())

	err := srvc.Listen()
	require.NoError(t, err)

	err = srvc.Listen()
	require.EqualError(t, err, "service already started")

	err = srvc.Close()
	require.NoError(t, err)

	err = srvc.Close()
	require.EqualError(t, err, "service not started")

	pool.Add(makeTx(t, 0))
	srvc = NewService(pool, badValidation{}, trie.NewTrie())
	err = srvc.Listen()
	require.NoError(t, err)

	srvc.closed.Wait()
}

func TestService_GetProof(t *testing.T) {
	srvc := &Service{
		epochs: []epoch{{store: trie.NewTrie()}},
	}

	pr, err := srvc.GetProof([]byte("A"))
	require.NoError(t, err)
	require.Equal(t, []byte("A"), pr.GetKey())

	srvc.epochs[0].block.root = []byte{1}
	_, err = srvc.GetProof([]byte("A"))
	require.EqualError(t, err,
		"couldn't create proof: mismatch block and share store root 0x01 != ")

	srvc.epochs[0].store = badStore{}
	_, err = srvc.GetProof([]byte("A"))
	require.EqualError(t, err, "couldn't read share: oops")
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

type badValidation struct{}

func (v badValidation) Validate(store.ReadWriteTrie, []tap.Transaction) (validation.Data, error) {
	return nil, xerrors.New("oops")
}

type badStore struct {
	store.Store
}

func (s badStore) GetShare([]byte) (store.Share, error) {
	return nil, xerrors.New("oops")
}
