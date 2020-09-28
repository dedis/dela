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
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/access/darc"
	"go.dedis.ch/dela/core/execution/baremetal"
	"go.dedis.ch/dela/core/execution/baremetal/viewchange"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/core/ordering/cosipbft/blockstore"
	"go.dedis.ch/dela/core/ordering/cosipbft/pbft"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/hashtree/binprefix"
	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/pool"
	poolimpl "go.dedis.ch/dela/core/txn/pool/gossip"
	"go.dedis.ch/dela/core/txn/pool/mem"
	"go.dedis.ch/dela/core/txn/signed"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/core/validation/simple"
	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/cosi/flatcosi"
	"go.dedis.ch/dela/cosi/threshold"
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

func TestService_Scenario_Basic(t *testing.T) {
	nodes, ro, clean := makeAuthority(t, 5)
	defer clean()

	signer := nodes[0].signer

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	initial := ro.Take(mino.RangeFilter(0, 4)).(crypto.CollectiveAuthority)

	err := nodes[0].service.Setup(ctx, initial)
	require.NoError(t, err)

	events := nodes[2].service.Watch(ctx)

	err = nodes[0].pool.Add(makeTx(t, 0, signer))
	require.NoError(t, err)

	evt := waitEvent(t, events)
	require.Equal(t, uint64(0), evt.Index)

	err = nodes[1].pool.Add(makeTx(t, 1, signer))
	require.NoError(t, err)

	evt = waitEvent(t, events)
	require.Equal(t, uint64(1), evt.Index)

	err = nodes[1].pool.Add(makeRosterTx(t, 2, ro, signer))
	require.NoError(t, err)

	evt = waitEvent(t, events)
	require.Equal(t, uint64(2), evt.Index)

	err = nodes[1].pool.Add(makeTx(t, 3, signer))
	require.NoError(t, err)

	evt = waitEvent(t, events)
	require.Equal(t, uint64(3), evt.Index)

	proof, err := nodes[0].service.GetProof(keyRoster[:])
	require.NoError(t, err)
	require.NotNil(t, proof.GetValue())

	require.Equal(t, keyRoster[:], proof.GetKey())
	require.NotNil(t, proof.GetValue())

	checkProof(t, proof.(Proof), nodes[0].service)
}

func TestService_Scenario_ViewChange(t *testing.T) {
	nodes, ro, clean := makeAuthority(t, 4)
	defer clean()

	for _, node := range nodes {
		// Short timeout to for the first round that we want to fail.
		node.service.timeoutRound = 50 * time.Millisecond
		// Long enough timeout so that any slow machine won't fail the test.
		node.service.timeoutRoundAfterFailure = 30 * time.Second
		node.service.timeoutViewchange = 30 * time.Second
	}

	// Simulate an issue with the leader transaction pool so that it does not
	// receive any of them.
	nodes[0].pool.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := nodes[1].service.Setup(ctx, ro)
	require.NoError(t, err)

	events := nodes[2].service.Watch(ctx)

	// Other nodes will detect a transaction but no block incoming => timeout
	err = nodes[1].pool.Add(makeTx(t, 0, nodes[1].signer))
	require.NoError(t, err)

	evt := waitEvent(t, events)
	require.Equal(t, uint64(0), evt.Index)
}

func TestService_New(t *testing.T) {
	param := ServiceParam{
		Mino:       fake.Mino{},
		Cosi:       flatcosi.NewFlat(fake.Mino{}, fake.NewAggregateSigner()),
		Tree:       fakeTree{},
		Validation: simple.NewService(nil, nil),
		Pool:       badPool{},
	}

	genesis := blockstore.NewGenesisStore()
	genesis.Set(types.Genesis{})

	opts := []ServiceOption{
		WithHashFactory(fake.NewHashFactory(&fake.Hash{})),
		WithGenesisStore(genesis),
		WithBlockStore(blockstore.NewInMemory()),
	}

	srvc, err := NewService(param, opts...)
	require.NoError(t, err)
	require.NotNil(t, srvc)

	<-srvc.closed

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
	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.genesis = blockstore.NewGenesisStore()
	srvc.access = fakeAccess{}

	rpc.Done()

	authority := fake.NewAuthority(3, fake.NewSigner)
	ctx := context.Background()

	err := srvc.Setup(ctx, authority)
	require.NoError(t, err)

	err = srvc.Setup(ctx, authority)
	require.EqualError(t, err,
		"creating genesis: set genesis failed: genesis block is already set")

	srvc.started = make(chan struct{})
	srvc.genesis = fakeGenesisStore{errGet: xerrors.New("oops")}
	err = srvc.Setup(ctx, authority)
	require.EqualError(t, err, "failed to read genesis: oops")

	srvc.started = make(chan struct{})
	srvc.genesis = blockstore.NewGenesisStore()
	srvc.rpc = fake.NewBadRPC()
	err = srvc.Setup(ctx, authority)
	require.EqualError(t, err, "sending genesis: fake error")

	srvc.started = make(chan struct{})
	srvc.genesis = blockstore.NewGenesisStore()
	rpc = fake.NewRPC()
	rpc.SendResponseWithError(fake.NewAddress(1), xerrors.New("oops"))
	srvc.rpc = rpc
	err = srvc.Setup(ctx, authority)
	require.EqualError(t, err, "one request failed: oops")
}

func TestService_Main(t *testing.T) {
	srvc := &Service{processor: newProcessor()}
	srvc.rosterFac = authority.NewFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})
	srvc.closing = make(chan struct{})
	srvc.closed = make(chan struct{})

	close(srvc.closing)

	err := srvc.main()
	require.NoError(t, err)

	srvc.tree = blockstore.NewTreeCache(fakeTree{err: xerrors.New("oops")})
	srvc.closing = make(chan struct{})
	srvc.started = make(chan struct{})
	srvc.closed = make(chan struct{})
	close(srvc.started)
	err = srvc.main()
	require.EqualError(t, err, "refreshing roster: reading roster: read from tree: oops")

	srvc.tree.Set(fakeTree{})
	srvc.pool = badPool{}
	srvc.closed = make(chan struct{})
	err = srvc.main()
	require.EqualError(t, err, "refreshing roster: updating tx pool: oops")

	logger, buffer := fake.WaitLog("round failed", 2*time.Second, func() { close(srvc.closing) })

	srvc.logger = logger
	srvc.pool = mem.NewPool()
	srvc.pbftsm = fakeSM{errLeader: xerrors.New("oops")}
	srvc.closed = make(chan struct{})
	err = srvc.main()
	require.NoError(t, err)
	require.Contains(t, buffer.String(), "round failed")
}

func TestService_DoRound(t *testing.T) {
	rpc := fake.NewRPC()
	ch := make(chan pbft.State)

	srvc := &Service{
		processor:                newProcessor(),
		me:                       fake.NewAddress(1),
		rpc:                      rpc,
		timeoutRound:             time.Millisecond,
		timeoutRoundAfterFailure: time.Millisecond,
		closing:                  make(chan struct{}),
	}
	srvc.blocks = blockstore.NewInMemory()
	srvc.sync = fakeSync{}
	srvc.pool = mem.NewPool()
	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.rosterFac = authority.NewFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})
	srvc.pbftsm = fakeSM{
		state: pbft.ViewChangeState,
		ch:    ch,
	}

	rpc.SendResponse(fake.NewAddress(3), nil)
	rpc.SendResponseWithError(fake.NewAddress(2), xerrors.New("oops"))
	rpc.Done()

	ctx := context.Background()

	// Round with timeout but no transaction in the pool.
	err := srvc.doRound(ctx)
	require.NoError(t, err)

	srvc.pool.Add(makeTx(t, 0, fake.NewSigner()))

	go func() {
		ch <- pbft.InitialState
		close(ch)
	}()

	// Round with timeout and a transaction in the pool.
	err = srvc.doRound(ctx)
	require.NoError(t, err)

	err = srvc.doRound(ctx)
	require.EqualError(t, err, "viewchange failed")

	srvc.pbftsm = fakeSM{err: xerrors.New("oops"), state: pbft.InitialState}
	err = srvc.doRound(ctx)
	require.EqualError(t, err, "pbft expire failed: oops")

	srvc.pbftsm = fakeSM{}
	srvc.rpc = fake.NewBadRPC()
	err = srvc.doRound(ctx)
	require.EqualError(t, err, "rpc failed: fake error")

	srvc.rosterFac = badRosterFac{}
	err = srvc.doRound(ctx)
	require.EqualError(t, err, "reading roster: decode failed: oops")

	srvc.rosterFac = authority.NewFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})
	srvc.pbftsm = fakeSM{errLeader: xerrors.New("oops")}
	err = srvc.doRound(ctx)
	require.EqualError(t, err, "reading leader: oops")

	srvc.pbftsm = fakeSM{}
	srvc.me = fake.NewAddress(0)
	srvc.sync = fakeSync{err: xerrors.New("oops")}
	err = srvc.doRound(ctx)
	require.EqualError(t, err, "sync failed: oops")

	srvc.sync = fakeSync{}
	srvc.val = fakeValidation{err: xerrors.New("oops")}
	err = srvc.doRound(ctx)
	require.EqualError(t, err,
		"pbft failed: failed to prepare data: staging tree failed: validation failed: oops")
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
	srvc.rosterFac = authority.NewFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})
	srvc.actor = fakeCosiActor{}
	srvc.pool = mem.NewPool()
	srvc.rpc = rpc

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rpc.SendResponseWithError(fake.NewAddress(5), xerrors.New("oops"))
	rpc.Done()
	srvc.genesis.Set(types.Genesis{})

	// Context timed out and no transaction are in the pool.
	err := srvc.doPBFT(ctx)
	require.NoError(t, err)

	// This time the gathering succeeds.
	ctx = context.Background()
	srvc.pool.Add(makeTx(t, 0, fake.NewSigner()))
	err = srvc.doPBFT(ctx)
	require.NoError(t, err)

	srvc.val = fakeValidation{err: xerrors.New("oops")}
	err = srvc.doPBFT(ctx)
	require.EqualError(t, err,
		"failed to prepare data: staging tree failed: validation failed: oops")

	srvc.val = fakeValidation{}
	srvc.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	err = srvc.doPBFT(ctx)
	require.EqualError(t, err,
		"creating block failed: fingerprint failed: couldn't write index: fake error")

	srvc.hashFactory = crypto.NewSha256Factory()
	srvc.pbftsm = fakeSM{err: xerrors.New("oops")}
	err = srvc.doPBFT(ctx)
	require.EqualError(t, err, "pbft prepare failed: oops")

	srvc.pbftsm = fakeSM{}
	srvc.tree.Set(fakeTree{err: xerrors.New("oops")})
	err = srvc.doPBFT(ctx)
	require.EqualError(t, err, "read roster failed: read from tree: oops")

	srvc.tree.Set(fakeTree{})
	srvc.actor = fakeCosiActor{err: xerrors.New("oops")}
	err = srvc.doPBFT(ctx)
	require.EqualError(t, err, "prepare phase failed: oops")

	srvc.actor = fakeCosiActor{err: xerrors.New("oops"), counter: fake.NewCounter(1)}
	err = srvc.doPBFT(ctx)
	require.EqualError(t, err, "commit phase failed: oops")

	srvc.actor = fakeCosiActor{}
	srvc.rpc = fake.NewBadRPC()
	err = srvc.doPBFT(ctx)
	require.EqualError(t, err, "rpc failed: fake error")

	srvc.rpc = rpc
	srvc.genesis = blockstore.NewGenesisStore()
	err = srvc.doPBFT(ctx)
	require.EqualError(t, err, "wake up failed: read genesis failed: missing genesis block")

	ctx, cancel = context.WithCancel(context.Background())
	cancel()

	err = srvc.doPBFT(ctx)
	require.EqualError(t, err, "context canceled")
}

func TestService_WakeUp(t *testing.T) {
	rpc := fake.NewRPC()

	srvc := &Service{processor: newProcessor()}
	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.genesis = blockstore.NewGenesisStore()
	srvc.genesis.Set(types.Genesis{})
	srvc.rosterFac = authority.NewFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})
	srvc.rpc = rpc

	ctx := context.Background()

	rpc.SendResponseWithError(fake.NewAddress(5), xerrors.New("oops"))
	rpc.Done()
	ro := authority.FromAuthority(fake.NewAuthority(3, fake.NewSigner))

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

func TestService_GetProof(t *testing.T) {
	srvc := &Service{processor: newProcessor()}
	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.blocks = blockstore.NewInMemory()
	srvc.blocks.Store(makeBlock(t, types.Digest{}))

	proof, err := srvc.GetProof([]byte("A"))
	require.NoError(t, err)
	require.NotNil(t, proof)

	srvc.tree.Set(fakeTree{err: xerrors.New("oops")})
	_, err = srvc.GetProof([]byte("A"))
	require.EqualError(t, err, "reading path: oops")

	srvc.tree.Set(fakeTree{})
	srvc.blocks = blockstore.NewInMemory()
	_, err = srvc.GetProof([]byte("A"))
	require.EqualError(t, err, "reading chain: store is empty")
}

func TestService_GetStore(t *testing.T) {
	srvc := &Service{processor: newProcessor()}
	srvc.tree = blockstore.NewTreeCache(fakeTree{})

	require.IsType(t, fakeTree{}, srvc.GetStore())
}

func TestService_GetRoster(t *testing.T) {
	srvc := &Service{processor: newProcessor()}
	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.rosterFac = fakeRosterFac{}

	roster, err := srvc.GetRoster()
	require.NoError(t, err)
	require.Equal(t, 3, roster.Len())
}

// -----------------------------------------------------------------------------
// Utility functions

func checkProof(t *testing.T, p Proof, s *Service) {
	genesis, err := s.genesis.Get()
	require.NoError(t, err)

	err = p.Verify(genesis, s.verifierFac)
	require.NoError(t, err)
}

type testNode struct {
	service *Service
	pool    *poolimpl.Pool
	db      kv.DB
	dbpath  string
	signer  crypto.Signer
}

const testContractName = "abc"

type testExec struct {
	err error
}

func (e testExec) Execute(txn.Transaction, store.Snapshot) error {
	return e.err
}

func makeTx(t *testing.T, nonce uint64, signer crypto.Signer) txn.Transaction {
	opts := []signed.TransactionOption{
		signed.WithArg(baremetal.ContractArg, []byte(testContractName)),
	}

	tx, err := signed.NewTransaction(nonce, signer.GetPublicKey(), opts...)
	require.NoError(t, err)

	require.NoError(t, tx.Sign(signer))

	return tx
}

func makeRosterTx(t *testing.T, nonce uint64, roster authority.Authority, signer crypto.Signer) txn.Transaction {
	data, err := roster.Serialize(json.NewContext())
	require.NoError(t, err)

	tx, err := signed.NewTransaction(
		nonce,
		signer.GetPublicKey(),
		signed.WithArg(baremetal.ContractArg, []byte(viewchange.ContractName)),
		signed.WithArg(viewchange.AuthorityArg, data),
	)
	require.NoError(t, err)

	require.NoError(t, tx.Sign(signer))

	return tx
}

func waitEvent(t *testing.T, events <-chan ordering.Event) ordering.Event {
	select {
	case <-time.After(15 * time.Second):
		t.Fatal("no event received before the timeout")
		return ordering.Event{}
	case evt := <-events:
		return evt
	}
}

func makeAuthority(t *testing.T, n int) ([]testNode, authority.Authority, func()) {
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

		c := threshold.NewCoSi(m, signer)
		c.SetThreshold(threshold.ByzantineThreshold)

		dir, err := ioutil.TempDir(os.TempDir(), "cosipbft")
		require.NoError(t, err)

		db, err := kv.New(filepath.Join(dir, "test.db"))
		require.NoError(t, err)

		txFac := signed.NewTransactionFactory()

		pool, err := poolimpl.NewPool(gossip.NewFlat(m, txFac))
		require.NoError(t, err)

		tree := binprefix.NewMerkleTree(db, binprefix.Nonce{})

		exec := baremetal.NewExecution()
		exec.Set(testContractName, testExec{})

		accessSrvc := darc.NewService(json.NewContext())

		rosterFac := authority.NewFactory(m.GetAddressFactory(), c.GetPublicKeyFactory())
		RegisterRosterContract(exec, rosterFac, accessSrvc)

		vs := simple.NewService(exec, txFac)

		param := ServiceParam{
			Mino:       m,
			Cosi:       c,
			Validation: vs,
			Access:     accessSrvc,
			Pool:       pool,
			Tree:       tree,
			DB:         db,
		}

		srv, err := NewService(param)
		require.NoError(t, err)

		nodes[i] = testNode{
			service: srv,
			pool:    pool,
			db:      db,
			dbpath:  dir,
			signer:  c.GetSigner(),
		}
	}

	ro := authority.New(addrs, pubkeys)

	clean := func() {
		for _, node := range nodes {
			require.NoError(t, node.service.Close())
			require.NoError(t, node.db.Close())
			require.NoError(t, os.RemoveAll(node.dbpath))
		}
	}

	return nodes, ro, clean
}

type badRosterFac struct {
	authority.Factory
}

func (fac badRosterFac) AuthorityOf(serde.Context, []byte) (authority.Authority, error) {
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

func (c badCosi) GetSigner() crypto.Signer {
	return fake.NewBadSigner()
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

type fakeRosterFac struct {
	authority.Factory
}

func (fakeRosterFac) AuthorityOf(serde.Context, []byte) (authority.Authority, error) {
	return authority.FromAuthority(fake.NewAuthority(3, fake.NewSigner)), nil
}

type fakeAccess struct {
	access.Service

	err error
}

func (srvc fakeAccess) Grant(store.Snapshot, access.Credential, ...access.Identity) error {
	return srvc.err
}
