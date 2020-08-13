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
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
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
	"go.dedis.ch/dela/cosi"
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

	err := srvs[0].service.Setup(ctx, initial)
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

func TestService_New(t *testing.T) {
	param := ServiceParam{
		Mino:       fake.Mino{},
		Cosi:       flatcosi.NewFlat(fake.Mino{}, fake.NewAggregateSigner()),
		Tree:       fakeTree{},
		Validation: simple.NewService(nil, anon.NewTransactionFactory()),
	}

	srvc, err := NewService(param, WithHashFactory(fake.NewHashFactory(&fake.Hash{})))
	require.NoError(t, err)
	require.NotNil(t, srvc)

	param.Mino = fake.NewBadMino()
	_, err = NewService(param)
	require.EqualError(t, err, "creating sync failed: rpc creation failed: fake error")

	param.Mino = fake.Mino{}
	param.Cosi = badCosi{}
	_, err = NewService(param)
	require.EqualError(t, err, "creating cosi failed: oops")
}

func TestService_Setup(t *testing.T) {
	rpc := fake.NewRPC()

	srvc := &Service{processor: newProcessor()}
	srvc.rpc = rpc
	srvc.hashFactory = crypto.NewSha256Factory()

	rpc.Done()

	authority := fake.NewAuthority(3, fake.NewSigner)
	ctx := context.Background()

	err := srvc.Setup(ctx, authority)
	require.NoError(t, err)

	srvc.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	err = srvc.Setup(ctx, authority)
	require.EqualError(t, err,
		"creating genesis: fingerprint failed: couldn't write root: fake error")

	srvc.hashFactory = crypto.NewSha256Factory()
	srvc.rpc = fake.NewBadRPC()
	err = srvc.Setup(ctx, authority)
	require.EqualError(t, err, "sending genesis: fake error")

	rpc = fake.NewRPC()
	rpc.SendResponseWithError(fake.NewAddress(1), xerrors.New("oops"))
	srvc.rpc = rpc
	err = srvc.Setup(ctx, authority)
	require.EqualError(t, err, "one request failed: oops")
}

func TestService_Main(t *testing.T) {
	srvc := &Service{processor: newProcessor()}
	srvc.rosterFac = roster.NewFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})
	srvc.closing = make(chan struct{})

	require.NoError(t, srvc.Close())

	err := srvc.main()
	require.NoError(t, err)

	srvc.tree = blockstore.NewTreeCache(fakeTree{err: xerrors.New("oops")})
	srvc.closing = make(chan struct{})
	srvc.started = make(chan struct{})
	close(srvc.started)
	err = srvc.main()
	require.EqualError(t, err, "refreshing roster: reading roster: read from tree: oops")

	srvc.tree.Set(fakeTree{})
	srvc.pool = badPool{}
	err = srvc.main()
	require.EqualError(t, err, "refreshing roster: updating tx pool: oops")

	srvc.pool = mem.NewPool()
	srvc.pbftsm = fakeSM{errLeader: xerrors.New("oops")}
	err = srvc.main()
	require.EqualError(t, err, "round failed: reading leader: oops")
}

func TestService_DoRound(t *testing.T) {
	rpc := fake.NewRPC()
	ch := make(chan pbft.State)

	srvc := &Service{
		processor: newProcessor(),
		me:        fake.NewAddress(1),
		rpc:       rpc,
		timeout:   time.Millisecond,
		closing:   make(chan struct{}),
	}
	srvc.blocks = blockstore.NewInMemory()
	srvc.sync = fakeSync{}
	srvc.pool = mem.NewPool()
	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.rosterFac = roster.NewFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})
	srvc.pbftsm = fakeSM{
		state: pbft.InitialState,
		ch:    ch,
	}

	rpc.SendResponse(fake.NewAddress(3), nil)
	rpc.SendResponseWithError(fake.NewAddress(2), xerrors.New("oops"))
	rpc.Done()

	srvc.pool.Add(makeTx(t, 0))

	go func() {
		ch <- pbft.InitialState
		close(ch)
		srvc.Close()
	}()

	err := srvc.doRound()
	require.NoError(t, err)

	srvc.closing = make(chan struct{})

	srvc.pbftsm = fakeSM{err: xerrors.New("oops"), state: pbft.InitialState}
	err = srvc.doRound()
	require.EqualError(t, err, "pbft expire failed: oops")

	srvc.pbftsm = fakeSM{}
	srvc.rpc = fake.NewBadRPC()
	err = srvc.doRound()
	require.EqualError(t, err, "rpc failed: fake error")

	srvc.rosterFac = badRosterFac{}
	err = srvc.doRound()
	require.EqualError(t, err, "reading roster: decode failed: oops")

	srvc.rosterFac = roster.NewFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})
	srvc.pbftsm = fakeSM{errLeader: xerrors.New("oops")}
	err = srvc.doRound()
	require.EqualError(t, err, "reading leader: oops")

	srvc.pbftsm = fakeSM{}
	srvc.me = fake.NewAddress(0)
	srvc.val = fakeValidation{err: xerrors.New("oops")}
	err = srvc.doRound()
	require.EqualError(t, err,
		"pbft failed: failed to prepare data: staging tree failed: validation failed: oops")
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

func TestService_DoPBFT(t *testing.T) {
	rpc := fake.NewRPC()

	srvc := &Service{processor: newProcessor()}
	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.val = fakeValidation{}
	srvc.blocks = blockstore.NewInMemory()
	srvc.genesis = blockstore.NewGenesisStore()
	srvc.hashFactory = crypto.NewSha256Factory()
	srvc.pbftsm = fakeSM{}
	srvc.rosterFac = roster.NewFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})
	srvc.actor = fakeCosiActor{}
	srvc.rpc = rpc

	ctx := context.Background()

	rpc.SendResponseWithError(fake.NewAddress(5), xerrors.New("oops"))
	rpc.Done()
	srvc.genesis.Set(types.Genesis{})

	err := srvc.doPBFT(ctx, nil)
	require.NoError(t, err)

	srvc.val = fakeValidation{err: xerrors.New("oops")}
	err = srvc.doPBFT(ctx, nil)
	require.EqualError(t, err,
		"failed to prepare data: staging tree failed: validation failed: oops")

	srvc.val = fakeValidation{}
	srvc.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	err = srvc.doPBFT(ctx, nil)
	require.EqualError(t, err,
		"creating block failed: fingerprint failed: couldn't write index: fake error")

	srvc.hashFactory = crypto.NewSha256Factory()
	srvc.pbftsm = fakeSM{err: xerrors.New("oops")}
	err = srvc.doPBFT(ctx, nil)
	require.EqualError(t, err, "pbft prepare failed: oops")

	srvc.pbftsm = fakeSM{}
	srvc.tree.Set(fakeTree{err: xerrors.New("oops")})
	err = srvc.doPBFT(ctx, nil)
	require.EqualError(t, err, "read roster failed: read from tree: oops")

	srvc.tree.Set(fakeTree{})
	srvc.actor = fakeCosiActor{err: xerrors.New("oops")}
	err = srvc.doPBFT(ctx, nil)
	require.EqualError(t, err, "prepare phase failed: oops")

	srvc.actor = fakeCosiActor{err: xerrors.New("oops"), counter: fake.NewCounter(1)}
	err = srvc.doPBFT(ctx, nil)
	require.EqualError(t, err, "commit phase failed: oops")

	srvc.actor = fakeCosiActor{}
	srvc.rpc = fake.NewBadRPC()
	err = srvc.doPBFT(ctx, nil)
	require.EqualError(t, err, "rpc failed: fake error")

	srvc.rpc = rpc
	srvc.genesis = blockstore.NewGenesisStore()
	err = srvc.doPBFT(ctx, nil)
	require.EqualError(t, err, "wake up failed: read genesis failed: missing genesis block")
}

func TestService_WakeUp(t *testing.T) {
	rpc := fake.NewRPC()

	srvc := &Service{processor: newProcessor()}
	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.genesis = blockstore.NewGenesisStore()
	srvc.genesis.Set(types.Genesis{})
	srvc.rosterFac = roster.NewFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})
	srvc.rpc = rpc

	ctx := context.Background()

	rpc.SendResponseWithError(fake.NewAddress(5), xerrors.New("oops"))
	rpc.Done()
	ro := roster.FromAuthority(fake.NewAuthority(3, fake.NewSigner))

	err := srvc.wakeUp(ctx, ro)
	require.NoError(t, err)

	srvc.tree.Set(fakeTree{err: xerrors.New("oops")})
	err = srvc.wakeUp(ctx, ro)
	require.EqualError(t, err, "read roster failed: read from tree: oops")

	srvc.tree.Set(fakeTree{})
	srvc.rpc = fake.NewBadRPC()
	err = srvc.wakeUp(ctx, ro)
	require.EqualError(t, err, "rpc failed: fake error")
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

type badPool struct {
	pool.Pool
}

func (p badPool) SetPlayers(mino.Players) error {
	return xerrors.New("oops")
}

type badCosi struct {
	cosi.CollectiveSigning
}

func (c badCosi) GetPublicKeyFactory() crypto.PublicKeyFactory {
	return fake.NewPublicKeyFactory(fake.PublicKey{})
}

func (c badCosi) GetSignatureFactory() crypto.SignatureFactory {
	return fake.NewSignatureFactory(fake.Signature{})
}

func (c badCosi) GetVerifierFactory() crypto.VerifierFactory {
	return fake.NewVerifierFactory(fake.Verifier{})
}

func (c badCosi) Listen(cosi.Reactor) (cosi.Actor, error) {
	return nil, xerrors.New("oops")
}

type fakeValidation struct {
	validation.Service

	err error
}

func (val fakeValidation) Validate(store.Snapshot, []txn.Transaction) (validation.Data, error) {
	return simple.NewData(nil), val.err
}

type fakeCosiActor struct {
	cosi.Actor

	counter *fake.Counter
	err     error
}

func (c fakeCosiActor) Sign(ctx context.Context, msg serde.Message,
	ca crypto.CollectiveAuthority) (crypto.Signature, error) {

	if c.counter.Done() {
		return fake.Signature{}, c.err
	}

	c.counter.Decrease()
	return fake.Signature{}, nil
}
