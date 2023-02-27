package pow

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/execution/native"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/hashtree"
	tree "go.dedis.ch/dela/core/store/hashtree/binprefix"
	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/core/txn"
	pool "go.dedis.ch/dela/core/txn/pool/mem"
	"go.dedis.ch/dela/core/txn/signed"
	"go.dedis.ch/dela/core/validation"
	val "go.dedis.ch/dela/core/validation/simple"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/internal/testing/fake"
	"golang.org/x/xerrors"
)

func TestService_Basic(t *testing.T) {
	tree, clean := makeTree(t)
	defer clean()

	exec := native.NewExecution()
	exec.Set(testContractName, testExec{})

	pool := pool.NewPool()
	srvc := NewService(
		pool,
		val.NewService(exec, signed.NewTransactionFactory()),
		tree,
	)

	// 1. Start the ordering service.
	require.NoError(t, srvc.Listen())
	defer srvc.Stop()

	// 2. Watch for new events before sending a transaction.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	evts := srvc.Watch(ctx)

	signer := bls.NewSigner()

	// 3. Send a transaction to the pool. It should be detected by the ordering
	// service and start a new block.
	require.NoError(t, pool.Add(makeTx(t, 0, signer)))

	evt := <-evts
	require.Equal(t, uint64(1), evt.Index)

	pr, err := srvc.GetProof([]byte("ping"))
	require.NoError(t, err)
	require.Equal(t, []byte("pong"), pr.GetValue())

	// 4. Send another transaction to the pool. This time it should creates a
	// block appended to the previous one.
	require.NoError(t, pool.Add(makeTx(t, 1, signer)))

	evt = <-evts
	require.Equal(t, uint64(2), evt.Index)
}

func TestService_Listen(t *testing.T) {
	tree, clean := makeTree(t)
	defer clean()

	vs := val.NewService(native.NewExecution(), signed.NewTransactionFactory())

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

	pool.Add(makeTx(t, 0, fake.NewSigner()))
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
	require.EqualError(t, err, fake.Err("couldn't read share"))
}

// -----------------------------------------------------------------------------
// Utility functions

const testContractName = "abc"

func makeTree(t *testing.T) (hashtree.Tree, func()) {
	dir, err := os.MkdirTemp(os.TempDir(), "dela-pow")
	require.NoError(t, err)

	db, err := kv.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)

	tree := tree.NewMerkleTree(db, tree.Nonce{})
	tree.Stage(func(store.Snapshot) error { return nil })

	return tree, func() { os.RemoveAll(dir) }
}

func makeTx(t *testing.T, nonce uint64, signer crypto.Signer) txn.Transaction {
	tx, err := signed.NewTransaction(
		nonce,
		signer.GetPublicKey(),
		signed.WithArg("key", []byte("ping")),
		signed.WithArg("value", []byte("pong")),
		signed.WithArg(native.ContractArg, []byte(testContractName)),
	)
	require.NoError(t, err)

	return tx
}

type testExec struct{}

func (e testExec) Execute(store store.Snapshot, step execution.Step) error {
	key := step.Current.GetArg("key")
	value := step.Current.GetArg("value")

	if len(key) == 0 || len(value) == 0 {
		return xerrors.New("key or value is nil")
	}

	err := store.Set(key, value)
	if err != nil {
		return err
	}

	return nil
}

type badValidation struct {
	validation.Service
}

func (v badValidation) Validate(store.Snapshot, []txn.Transaction) (validation.Result, error) {
	return nil, fake.GetError()
}

type badTrie struct {
	hashtree.Tree
}

func (s badTrie) GetPath([]byte) (hashtree.Path, error) {
	return nil, fake.GetError()
}
