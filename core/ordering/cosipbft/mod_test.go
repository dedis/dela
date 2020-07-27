package cosipbft

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/execution/baremetal"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/hashtree/binprefix"
	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/core/tap"
	txn "go.dedis.ch/dela/core/tap/anon"
	"go.dedis.ch/dela/core/tap/pool"
	poolimpl "go.dedis.ch/dela/core/tap/pool/gossip"
	"go.dedis.ch/dela/core/validation/simple"
	"go.dedis.ch/dela/cosi/flatcosi"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/gossip"
	"go.dedis.ch/dela/mino/minoch"
)

func TestService_Basic(t *testing.T) {
	srvs, ro, clean := makeAuthority(t, 3)
	defer clean()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srvs[0].service.Setup(ro)
	require.NoError(t, err)

	events := srvs[2].service.Watch(ctx)

	err = srvs[0].pool.Add(makeTx(t, 0))
	require.NoError(t, err)

	evt := waitEvent(t, events)
	require.Equal(t, uint64(1), evt.Index)

	err = srvs[1].pool.Add(makeTx(t, 1))
	require.NoError(t, err)

	evt = waitEvent(t, events)
	require.Equal(t, uint64(1), evt.Index)
}

// Utility functions -----------------------------------------------------------

type testNode struct {
	service *Service
	pool    pool.Pool
	dbpath  string
}

type testExec struct {
	err error
}

func (e testExec) Execute(tap.Transaction, store.Snapshot) (execution.Result, error) {
	return execution.Result{Accepted: true}, e.err
}

func makeTx(t *testing.T, nonce uint64) tap.Transaction {
	tx, err := txn.NewTransaction(nonce)
	require.NoError(t, err)
	return tx
}

func waitEvent(t *testing.T, events <-chan ordering.Event) ordering.Event {
	select {
	case <-time.After(5 * time.Second):
		t.Fatal("no event received before the timeout")
		return ordering.Event{}
	case evt := <-events:
		return evt
	}
}

func makeAuthority(t *testing.T, n int) ([]testNode, crypto.CollectiveAuthority, func()) {
	manager := minoch.NewManager()

	addrs := make([]mino.Address, n)
	pubkeys := make([]crypto.PublicKey, n)
	nodes := make([]testNode, n)

	for i := 0; i < n; i++ {
		m, err := minoch.NewMinoch(manager, fmt.Sprintf("node%d", i))
		require.NoError(t, err)

		addrs[i] = m.GetAddress()

		signer := bls.NewSigner()
		pubkeys[i] = signer.GetPublicKey()

		c := flatcosi.NewFlat(m, signer)

		dir, err := ioutil.TempDir(os.TempDir(), "cosipbft")
		require.NoError(t, err)

		db, err := kv.New(filepath.Join(dir, "test.db"))
		require.NoError(t, err)

		pool, err := poolimpl.NewPool(gossip.NewFlat(m, txn.NewTransactionFactory()))
		require.NoError(t, err)

		tree := binprefix.NewMerkleTree(db, binprefix.Nonce{})

		vs := simple.NewService(baremetal.NewExecution(testExec{}), txn.NewTransactionFactory())

		srv, err := NewService(m, c, pool, tree, vs)
		require.NoError(t, err)

		nodes[i] = testNode{
			service: srv,
			pool:    pool,
			dbpath:  dir,
		}
	}

	ro := roster.New(addrs, pubkeys)

	clean := func() {
		for _, node := range nodes {
			node.service.Close()

			os.RemoveAll(node.dbpath)
		}
	}

	return nodes, ro, clean
}
