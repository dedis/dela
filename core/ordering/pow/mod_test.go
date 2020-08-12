package pow

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/execution/baremetal"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/hashtree"
	tree "go.dedis.ch/dela/core/store/hashtree/binprefix"
	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/anon"
	pool "go.dedis.ch/dela/core/txn/pool/mem"
	"go.dedis.ch/dela/core/validation"
	val "go.dedis.ch/dela/core/validation/simple"
	"golang.org/x/xerrors"
)

func TestService_Basic(t *testing.T) {
	tree, clean := makeTree(t)
	defer clean()

	pool := pool.NewPool()
	srvc := NewService(
		pool,
		val.NewService(baremetal.NewExecution(testExec{}), anon.NewTransactionFactory()),
		tree,
	)

	// 1. Start the ordering service.
	require.NoError(t, srvc.Listen())
	defer srvc.Stop()

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
	tree, clean := makeTree(t)
	defer clean()

	vs := val.NewService(baremetal.NewExecution(testExec{}), anon.NewTransactionFactory())

	pool := pool.NewPool()
	srvc := NewService(pool, vs, tree)

	err := srvc.Listen()
	require.NoError(t, err)

	err = srvc.Listen()
	require.EqualError(t, err, "service already started")

	err = srvc.Stop()
	require.NoError(t, err)

	err = srvc.Stop()
	require.EqualError(t, err, "service not started")

	pool.Add(makeTx(t, 0))
	srvc = NewService(pool, badValidation{}, tree)
	err = srvc.Listen()
	require.NoError(t, err)

	srvc.closed.Wait()
}

func TestService_GetProof(t *testing.T) {
	tree, clean := makeTree(t)
	defer clean()

	srvc := &Service{
		epochs: []epoch{{store: tree}},
	}

	pr, err := srvc.GetProof([]byte("A"))
	require.NoError(t, err)
	require.Equal(t, []byte("A"), pr.GetKey())

	srvc.epochs[0].block.root = []byte{1}
	_, err = srvc.GetProof([]byte("A"))
	require.EqualError(t, err,
		"couldn't create proof: mismatch block and share store root 0x01 != ")

	srvc.epochs[0].store = badTrie{}
	_, err = srvc.GetProof([]byte("A"))
	require.EqualError(t, err, "couldn't read share: oops")
}

// -----------------------------------------------------------------------------
// Utility functions

func makeTree(t *testing.T) (hashtree.Tree, func()) {
	dir, err := ioutil.TempDir(os.TempDir(), "dela-pow")
	require.NoError(t, err)

	db, err := kv.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)

	tree := tree.NewMerkleTree(db, tree.Nonce{})
	tree.Stage(func(store.Snapshot) error { return nil })

	return tree, func() { os.RemoveAll(dir) }
}

func makeTx(t *testing.T, nonce uint64) txn.Transaction {
	tx, err := anon.NewTransaction(nonce, anon.WithArg("key", []byte("ping")), anon.WithArg("value", []byte("pong")))
	require.NoError(t, err)

	return tx
}

type testExec struct{}

func (e testExec) Execute(tx txn.Transaction, store store.Snapshot) (execution.Result, error) {
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

type badValidation struct {
	validation.Service
}

func (v badValidation) Validate(store.Snapshot, []txn.Transaction) (validation.Data, error) {
	return nil, xerrors.New("oops")
}

type badTrie struct {
	hashtree.Tree
}

func (s badTrie) GetPath([]byte) (hashtree.Path, error) {
	return nil, xerrors.New("oops")
}
