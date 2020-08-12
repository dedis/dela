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
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/execution/baremetal"
	rosterchange "go.dedis.ch/dela/core/execution/baremetal/viewchange"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/ordering/cosipbft/blockstore"
	"go.dedis.ch/dela/core/ordering/cosipbft/pbft"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/hashtree/binprefix"
	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/anon"
	"go.dedis.ch/dela/core/txn/pool"
	poolimpl "go.dedis.ch/dela/core/txn/pool/gossip"
	"go.dedis.ch/dela/core/txn/pool/mem"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/core/validation/simple"
	"go.dedis.ch/dela/cosi/flatcosi"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/gossip"
	"go.dedis.ch/dela/mino/minoch"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
	"golang.org/x/xerrors"
)

func TestService_Basic(t *testing.T) {
	srvs, ro, clean := makeAuthority(t, 5)
	defer clean()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	initial := ro.Take(mino.RangeFilter(0, 4)).(crypto.CollectiveAuthority)

	err := srvs[0].service.Setup(initial)
	require.NoError(t, err)

	events := srvs[2].service.Watch(ctx)

	err = srvs[0].pool.Add(makeTx(t, 0))
	require.NoError(t, err)

	evt := waitEvent(t, events)
	require.Equal(t, uint64(1), evt.Index)

	err = srvs[1].pool.Add(makeTx(t, 1))
	require.NoError(t, err)

	evt = waitEvent(t, events)
	require.Equal(t, uint64(2), evt.Index)

	err = srvs[1].pool.Add(makeRosterTx(t, 2, ro))
	require.NoError(t, err)

	evt = waitEvent(t, events)
	require.Equal(t, uint64(3), evt.Index)

	err = srvs[1].pool.Add(makeTx(t, 3))
	require.NoError(t, err)

	evt = waitEvent(t, events)
	require.Equal(t, uint64(4), evt.Index)
}

func TestService_ViewChange_DoRound(t *testing.T) {
	rpc := fake.NewRPC()
	ch := make(chan pbft.State)

	srvc := &Service{
		processor: newProcessor(),
		me:        fake.NewAddress(1),
		rpc:       rpc,
		timeout:   time.Millisecond,
		closing:   make(chan struct{}),
	}
	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.rosterFac = roster.NewFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})
	srvc.pbftsm = fakeSM{
		state: pbft.InitialState,
		ch:    ch,
	}

	rpc.SendResponse(fake.NewAddress(3), nil)
	rpc.SendResponseWithError(fake.NewAddress(2), xerrors.New("oops"))
	rpc.Done()

	go func() {
		ch <- pbft.PrePrepareState
		close(ch)
		srvc.Close()
	}()

	err := srvc.doRound()
	require.NoError(t, err)

	srvc.closing = make(chan struct{})
	srvc.pbftsm = fakeSM{err: xerrors.New("oops")}
	err = srvc.doRound()
	require.EqualError(t, err, "pbft pre-prepare failed: oops")

	srvc.pbftsm = fakeSM{err: xerrors.New("oops"), state: pbft.PrePrepareState}
	err = srvc.doRound()
	require.EqualError(t, err, "pbft expire failed: oops")

	srvc.pbftsm = fakeSM{}
	srvc.rpc = fake.NewBadRPC()
	err = srvc.doRound()
	require.EqualError(t, err, "rpc failed: fake error")

	srvc.rosterFac = badRosterFac{}
	err = srvc.doRound()
	require.EqualError(t, err, "read roster failed: decode failed: oops")
}

func TestService_CollectTxs(t *testing.T) {
	srvc := &Service{processor: newProcessor()}
	srvc.pool = mem.NewPool()

	go func() {
		time.Sleep(10 * time.Millisecond)
		srvc.pool.Add(makeTx(t, 0))
	}()

	txs := srvc.collectTxs(context.Background())
	require.Len(t, txs, 1)

	srvc.pool.Add(makeTx(t, 1))

	txs = srvc.collectTxs(context.Background())
	require.Len(t, txs, 2)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	srvc.pool = mem.NewPool()
	txs = srvc.collectTxs(ctx)
	require.Len(t, txs, 0)
}

func TestService_PrepareBlock(t *testing.T) {
	srvc := &Service{processor: newProcessor()}
	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.val = fakeValidation{}
	srvc.blocks = blockstore.NewInMemory()
	srvc.hashFactory = crypto.NewSha256Factory()

	block, err := srvc.prepareBlock(nil)
	require.NoError(t, err)
	require.NotNil(t, block)

	srvc.val = fakeValidation{err: xerrors.New("oops")}
	_, err = srvc.prepareBlock(nil)
	require.EqualError(t, err, "staging tree failed: validation failed: oops")

	srvc.val = fakeValidation{}
	srvc.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	_, err = srvc.prepareBlock(nil)
	require.EqualError(t, err,
		"creating block failed: fingerprint failed: couldn't write index: fake error")
}

// Utility functions -----------------------------------------------------------

type testNode struct {
	service *Service
	pool    pool.Pool
	dbpath  string
}

const testContractName = "abc"

type testExec struct {
	err error
}

func (e testExec) Execute(txn.Transaction, store.Snapshot) (execution.Result, error) {
	return execution.Result{Accepted: true}, e.err
}

func makeTx(t *testing.T, nonce uint64) txn.Transaction {
	tx, err := anon.NewTransaction(nonce, anon.WithArg(baremetal.ContractArg, []byte(testContractName)))
	require.NoError(t, err)
	return tx
}

func makeRosterTx(t *testing.T, nonce uint64, roster viewchange.Authority) txn.Transaction {
	data, err := roster.Serialize(json.NewContext())
	require.NoError(t, err)

	tx, err := anon.NewTransaction(
		nonce,
		anon.WithArg(baremetal.ContractArg, []byte(rosterchange.ContractName)),
		anon.WithArg(rosterchange.AuthorityArg, data),
	)
	require.NoError(t, err)

	return tx
}

func waitEvent(t *testing.T, events <-chan ordering.Event) ordering.Event {
	select {
	case <-time.After(4 * time.Second):
		t.Fatal("no event received before the timeout")
		return ordering.Event{}
	case evt := <-events:
		return evt
	}
}

func makeAuthority(t *testing.T, n int) ([]testNode, viewchange.Authority, func()) {
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

		pool, err := poolimpl.NewPool(gossip.NewFlat(m, anon.NewTransactionFactory()))
		require.NoError(t, err)

		tree := binprefix.NewMerkleTree(db, binprefix.Nonce{})

		exec := baremetal.NewExecution()
		exec.Set(testContractName, testExec{})

		rosterFac := roster.NewFactory(m.GetAddressFactory(), c.GetPublicKeyFactory())
		exec.Set(rosterchange.ContractName, rosterchange.NewContract(keyRoster[:], rosterFac))

		vs := simple.NewService(exec, anon.NewTransactionFactory())

		param := ServiceParam{
			Mino:       m,
			Cosi:       c,
			Validation: vs,
			Pool:       pool,
			Tree:       tree,
		}

		srv, err := NewService(param)
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

type badRosterFac struct {
	viewchange.AuthorityFactory
}

func (fac badRosterFac) AuthorityOf(serde.Context, []byte) (viewchange.Authority, error) {
	return nil, xerrors.New("oops")
}

type fakeValidation struct {
	validation.Service

	err error
}

func (val fakeValidation) Validate(store.Snapshot, []txn.Transaction) (validation.Data, error) {
	return simple.NewData(nil), val.err
}
