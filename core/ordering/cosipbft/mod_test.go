package cosipbft

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/access/darc"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/execution/native"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/core/ordering/cosipbft/blockstore"
	"go.dedis.ch/dela/core/ordering/cosipbft/contracts/viewchange"
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

	for i := 0; i < 3; i++ {
		err = nodes[1].pool.Add(makeTx(t, uint64(i+3), signer))
		require.NoError(t, err)

		evt = waitEvent(t, events)
		require.Equal(t, uint64(i+3), evt.Index)
	}

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

// Test that a block committed will be eventually finalized even if the
// propagation failed.
//
// Expected log warnings and errors:
//  - timeout from the followers
//  - block not from the leader
//  - round failed on node 0
//  - mismatch state viewchange != (initial|prepare)
func TestService_Scenario_FinalizeFailure(t *testing.T) {
	nodes, ro, clean := makeAuthority(t, 4)
	defer clean()

	filter := func(req mino.Request) bool {
		switch req.Message.(type) {
		case types.DoneMessage:
			// Ignore propagation from node 0 which produces a committed block
			// without finalization. Node 1 will take over and finalize it.
			return !req.Address.Equal(nodes[0].service.me)
		default:
			return true
		}
	}

	for i := 0; i < 4; i++ {
		nodes[i].onet.AddFilter(filter)
		nodes[i].service.timeoutRound = 200 * time.Millisecond
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := nodes[0].service.Setup(ctx, ro)
	require.NoError(t, err)

	events := nodes[1].service.Watch(ctx)

	err = nodes[0].pool.Add(makeTx(t, 0, nodes[0].signer))
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

	param.Cosi = badCosi{}
	_, err = NewService(param)
	require.EqualError(t, err, fake.Err("creating cosi failed"))
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

	_, more := <-srvc.started
	require.False(t, more)

	genesis, err := srvc.genesis.Get()
	require.NoError(t, err)
	require.Equal(t, 3, genesis.GetRoster().Len())
}

func TestService_AlreadySet_Setup(t *testing.T) {
	srvc := &Service{
		processor: newProcessor(),
	}

	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.access = fakeAccess{}
	srvc.genesis = blockstore.NewGenesisStore()
	srvc.genesis.Set(types.Genesis{})

	authority := fake.NewAuthority(3, fake.NewSigner)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srvc.Setup(ctx, authority)
	require.EqualError(t, err,
		"creating genesis: set genesis failed: genesis block is already set")
}

func TestService_FailReadGenesis_Setup(t *testing.T) {
	srvc := &Service{
		processor: newProcessor(),
	}

	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.access = fakeAccess{}
	srvc.genesis = fakeGenesisStore{errGet: fake.GetError()}

	authority := fake.NewAuthority(3, fake.NewSigner)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srvc.Setup(ctx, authority)
	require.EqualError(t, err, fake.Err("failed to read genesis"))
}

func TestService_FailPropagate_Setup(t *testing.T) {
	srvc := &Service{
		processor: newProcessor(),
	}

	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.access = fakeAccess{}
	srvc.genesis = blockstore.NewGenesisStore()
	srvc.rpc = fake.NewBadRPC()

	authority := fake.NewAuthority(3, fake.NewSigner)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srvc.Setup(ctx, authority)
	require.EqualError(t, err, fake.Err("sending genesis"))
}

func TestService_RequestFailure_Setup(t *testing.T) {
	srvc := &Service{
		processor: newProcessor(),
	}

	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.access = fakeAccess{}
	srvc.genesis = blockstore.NewGenesisStore()

	rpc := fake.NewRPC()
	rpc.SendResponseWithError(fake.NewAddress(1), fake.GetError())
	srvc.rpc = rpc

	authority := fake.NewAuthority(3, fake.NewSigner)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srvc.Setup(ctx, authority)
	require.EqualError(t, err, fake.Err("one request failed"))
}

func TestService_Main(t *testing.T) {
	srvc := &Service{processor: newProcessor()}
	srvc.rosterFac = authority.NewFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})
	srvc.closing = make(chan struct{})
	srvc.closed = make(chan struct{})

	close(srvc.closing)

	err := srvc.main()
	require.NoError(t, err)

	srvc.tree = blockstore.NewTreeCache(fakeTree{err: fake.GetError()})
	srvc.closing = make(chan struct{})
	srvc.started = make(chan struct{})
	srvc.closed = make(chan struct{})
	close(srvc.started)
	err = srvc.main()
	require.EqualError(t, err, fake.Err("refreshing roster: reading roster: read from tree"))

	srvc.tree.Set(fakeTree{})
	srvc.pool = badPool{}
	srvc.closed = make(chan struct{})
	err = srvc.main()
	require.EqualError(t, err, fake.Err("refreshing roster: updating tx pool"))

	logger, wait := fake.WaitLog("round failed", 2*time.Second)
	go func() {
		wait(t)
		close(srvc.closing)
	}()

	srvc.logger = logger
	srvc.pool = mem.NewPool()
	srvc.pbftsm = fakeSM{errLeader: fake.GetError()}
	srvc.closed = make(chan struct{})
	err = srvc.main()
	require.NoError(t, err)
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
	rpc.SendResponseWithError(fake.NewAddress(2), fake.GetError())
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
}

func TestService_ViewchangeFailed_DoRound(t *testing.T) {
	pbftsm := fakeSM{
		state: pbft.ViewChangeState,
		ch:    make(chan pbft.State),
	}
	// Stuck to view change state thus causing the view to fail.
	close(pbftsm.ch)

	rpc := fake.NewRPC()
	rpc.Done()

	srvc := &Service{
		processor:                newProcessor(),
		me:                       fake.NewAddress(1),
		rpc:                      rpc,
		timeoutRound:             time.Millisecond,
		timeoutRoundAfterFailure: time.Millisecond,
	}

	srvc.blocks = blockstore.NewInMemory()
	srvc.pool = mem.NewPool()
	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.rosterFac = authority.NewFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})
	srvc.pbftsm = pbftsm

	srvc.pool.Add(makeTx(t, 0, fake.NewSigner()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srvc.doRound(ctx)
	require.EqualError(t, err, "viewchange failed")
}

func TestService_FailPBFTExpire_DoRound(t *testing.T) {
	rpc := fake.NewRPC()
	rpc.Done()

	srvc := &Service{
		processor:                newProcessor(),
		me:                       fake.NewAddress(1),
		rpc:                      rpc,
		timeoutRound:             time.Millisecond,
		timeoutRoundAfterFailure: time.Millisecond,
	}

	srvc.blocks = blockstore.NewInMemory()
	srvc.pool = mem.NewPool()
	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.rosterFac = authority.NewFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})
	srvc.pbftsm = fakeSM{
		err:   fake.GetError(),
		state: pbft.InitialState,
	}

	srvc.pool.Add(makeTx(t, 0, fake.NewSigner()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srvc.doRound(ctx)
	require.EqualError(t, err, fake.Err("pbft expire failed"))
}

func TestService_FailSendViews_DoRound(t *testing.T) {
	srvc := &Service{
		processor:                newProcessor(),
		me:                       fake.NewAddress(1),
		rpc:                      fake.NewBadRPC(),
		timeoutRound:             time.Millisecond,
		timeoutRoundAfterFailure: time.Millisecond,
	}

	srvc.blocks = blockstore.NewInMemory()
	srvc.pool = mem.NewPool()
	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.rosterFac = authority.NewFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})
	srvc.pbftsm = fakeSM{}

	srvc.pool.Add(makeTx(t, 0, fake.NewSigner()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srvc.doRound(ctx)
	require.EqualError(t, err, fake.Err("rpc failed to send views"))
}

func TestService_FailReadRoster_DoRound(t *testing.T) {
	srvc := &Service{
		processor:                newProcessor(),
		me:                       fake.NewAddress(1),
		timeoutRound:             time.Millisecond,
		timeoutRoundAfterFailure: time.Millisecond,
	}

	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.rosterFac = badRosterFac{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srvc.doRound(ctx)
	require.EqualError(t, err, fake.Err("reading roster: decode failed"))
}

func TestService_FailReadLeader_DoRound(t *testing.T) {
	srvc := &Service{
		processor:                newProcessor(),
		me:                       fake.NewAddress(1),
		timeoutRound:             time.Millisecond,
		timeoutRoundAfterFailure: time.Millisecond,
	}

	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.rosterFac = authority.NewFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})
	srvc.pbftsm = fakeSM{errLeader: fake.GetError()}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srvc.doRound(ctx)
	require.EqualError(t, err, fake.Err("reading leader"))
}

func TestService_FailSync_DoRound(t *testing.T) {
	srvc := &Service{
		processor:                newProcessor(),
		me:                       fake.NewAddress(0),
		timeoutRound:             time.Millisecond,
		timeoutRoundAfterFailure: time.Millisecond,
	}

	srvc.blocks = blockstore.NewInMemory()
	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.rosterFac = authority.NewFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})
	srvc.pbftsm = fakeSM{}
	srvc.sync = fakeSync{err: fake.GetError()}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srvc.doRound(ctx)
	require.EqualError(t, err, fake.Err("sync failed"))
}

func TestService_FailPBFT_DoRound(t *testing.T) {
	srvc := &Service{
		processor:                newProcessor(),
		me:                       fake.NewAddress(0),
		timeoutRound:             RoundTimeout,
		timeoutRoundAfterFailure: RoundTimeout,
		val:                      fakeValidation{err: fake.GetError()},
	}

	srvc.blocks = blockstore.NewInMemory()
	srvc.pool = mem.NewPool()
	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.rosterFac = authority.NewFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})
	srvc.pbftsm = fakeSM{}
	srvc.sync = fakeSync{}

	srvc.pool.Add(makeTx(t, 0, fake.NewSigner()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srvc.doRound(ctx)
	require.EqualError(t, err,
		fake.Err("pbft failed: failed to prepare data: staging tree failed: validation failed"))
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

	rpc.SendResponseWithError(fake.NewAddress(5), fake.GetError())
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
}

func TestService_ContextCanceld_DoPBFT(t *testing.T) {
	srvc := &Service{processor: newProcessor()}
	srvc.val = fakeValidation{err: fake.GetError()}
	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.pbftsm = fakeSM{}
	srvc.pool = mem.NewPool()

	srvc.pool.Add(makeTx(t, 0, fake.NewSigner()))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := srvc.doPBFT(ctx)
	require.EqualError(t, err, "context canceled")
}

func TestService_FailValidation_DoPBFT(t *testing.T) {
	srvc := &Service{processor: newProcessor()}
	srvc.val = fakeValidation{err: fake.GetError()}
	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.pbftsm = fakeSM{}
	srvc.pool = mem.NewPool()

	srvc.pool.Add(makeTx(t, 0, fake.NewSigner()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srvc.val = fakeValidation{err: fake.GetError()}
	err := srvc.doPBFT(ctx)
	require.EqualError(t, err,
		fake.Err("failed to prepare data: staging tree failed: validation failed"))
}

func TestService_FailCreateBlock_DoPBFT(t *testing.T) {
	srvc := &Service{processor: newProcessor()}
	srvc.val = fakeValidation{}
	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.pbftsm = fakeSM{}
	srvc.pool = mem.NewPool()
	srvc.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	srvc.blocks = blockstore.NewInMemory()

	srvc.pool.Add(makeTx(t, 0, fake.NewSigner()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srvc.doPBFT(ctx)
	require.EqualError(t, err,
		fake.Err("creating block failed: fingerprint failed: couldn't write index"))
}

func TestService_FailPrepare_DoPBFT(t *testing.T) {
	srvc := &Service{processor: newProcessor()}
	srvc.val = fakeValidation{}
	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.pbftsm = fakeSM{err: fake.GetError()}
	srvc.pool = mem.NewPool()
	srvc.hashFactory = crypto.NewSha256Factory()
	srvc.blocks = blockstore.NewInMemory()

	srvc.pool.Add(makeTx(t, 0, fake.NewSigner()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srvc.doPBFT(ctx)
	require.EqualError(t, err, fake.Err("pbft prepare failed"))
}

func TestService_FailReadRoster_DoPBFT(t *testing.T) {
	srvc := &Service{processor: newProcessor()}
	srvc.val = fakeValidation{}
	srvc.tree = blockstore.NewTreeCache(fakeTree{err: fake.GetError()})
	srvc.pbftsm = fakeSM{}
	srvc.pool = mem.NewPool()
	srvc.hashFactory = crypto.NewSha256Factory()
	srvc.blocks = blockstore.NewInMemory()

	srvc.pool.Add(makeTx(t, 0, fake.NewSigner()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srvc.doPBFT(ctx)
	require.EqualError(t, err, fake.Err("read roster failed: read from tree"))
}

func TestService_FailPrepareSig_DoPBFT(t *testing.T) {
	srvc := &Service{processor: newProcessor()}
	srvc.val = fakeValidation{}
	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.pbftsm = fakeSM{}
	srvc.pool = mem.NewPool()
	srvc.hashFactory = crypto.NewSha256Factory()
	srvc.blocks = blockstore.NewInMemory()
	srvc.actor = fakeCosiActor{err: fake.GetError()}
	srvc.rosterFac = authority.NewFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})

	srvc.pool.Add(makeTx(t, 0, fake.NewSigner()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srvc.doPBFT(ctx)
	require.EqualError(t, err, fake.Err("prepare signature failed"))
}

func TestService_FailCommitSign_DoPBFT(t *testing.T) {
	srvc := &Service{processor: newProcessor()}
	srvc.val = fakeValidation{}
	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.pbftsm = fakeSM{}
	srvc.pool = mem.NewPool()
	srvc.hashFactory = crypto.NewSha256Factory()
	srvc.blocks = blockstore.NewInMemory()
	srvc.actor = fakeCosiActor{
		err:     fake.GetError(),
		counter: fake.NewCounter(1),
	}
	srvc.rosterFac = authority.NewFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})

	srvc.pool.Add(makeTx(t, 0, fake.NewSigner()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srvc.doPBFT(ctx)
	require.EqualError(t, err, fake.Err("commit signature failed"))
}

func TestService_FailPropagation_DoPBFT(t *testing.T) {
	srvc := &Service{processor: newProcessor()}
	srvc.val = fakeValidation{}
	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.pbftsm = fakeSM{}
	srvc.pool = mem.NewPool()
	srvc.hashFactory = crypto.NewSha256Factory()
	srvc.blocks = blockstore.NewInMemory()
	srvc.actor = fakeCosiActor{}
	srvc.rosterFac = authority.NewFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})
	srvc.rpc = fake.NewBadRPC()

	srvc.pool.Add(makeTx(t, 0, fake.NewSigner()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srvc.doPBFT(ctx)
	require.EqualError(t, err, fake.Err("propagation failed"))
}

func TestService_FailWakeUp_DoPBFT(t *testing.T) {
	rpc := fake.NewRPC()
	rpc.Done()

	srvc := &Service{processor: newProcessor()}
	srvc.val = fakeValidation{}
	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.pbftsm = fakeSM{}
	srvc.pool = mem.NewPool()
	srvc.hashFactory = crypto.NewSha256Factory()
	srvc.blocks = blockstore.NewInMemory()
	srvc.actor = fakeCosiActor{}
	srvc.rosterFac = authority.NewFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})
	srvc.rpc = rpc
	srvc.genesis = blockstore.NewGenesisStore()

	srvc.pool.Add(makeTx(t, 0, fake.NewSigner()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srvc.doPBFT(ctx)
	require.EqualError(t, err, "wake up failed: read genesis failed: missing genesis block")
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

	rpc.SendResponseWithError(fake.NewAddress(5), fake.GetError())
	rpc.Done()
	ro := authority.FromAuthority(fake.NewAuthority(3, fake.NewSigner))

	err := srvc.wakeUp(ctx, ro)
	require.NoError(t, err)

	srvc.tree.Set(fakeTree{err: fake.GetError()})
	err = srvc.wakeUp(ctx, ro)
	require.EqualError(t, err, fake.Err("read roster failed: read from tree"))

	srvc.tree.Set(fakeTree{})
	srvc.rpc = fake.NewBadRPC()
	err = srvc.wakeUp(ctx, ro)
	require.EqualError(t, err, fake.Err("rpc failed"))
}

func TestService_GetProof(t *testing.T) {
	srvc := &Service{processor: newProcessor()}
	srvc.tree = blockstore.NewTreeCache(fakeTree{})
	srvc.blocks = blockstore.NewInMemory()
	srvc.blocks.Store(makeBlock(t, types.Digest{}))

	proof, err := srvc.GetProof([]byte("A"))
	require.NoError(t, err)
	require.NotNil(t, proof)

	srvc.tree.Set(fakeTree{err: fake.GetError()})
	_, err = srvc.GetProof([]byte("A"))
	require.EqualError(t, err, fake.Err("reading path"))

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

func TestService_PoolFilter(t *testing.T) {
	filter := poolFilter{
		tree: blockstore.NewTreeCache(fakeTree{}),
		srvc: fakeValidation{},
	}

	err := filter.Accept(makeTx(t, 0, fake.NewSigner()), validation.Leeway{})
	require.NoError(t, err)

	filter.srvc = fakeValidation{err: fake.GetError()}
	err = filter.Accept(makeTx(t, 0, fake.NewSigner()), validation.Leeway{})
	require.EqualError(t, err, fake.Err("unacceptable transaction"))
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
	onet    *minoch.Minoch
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

func (e testExec) Execute(store.Snapshot, execution.Step) error {
	return e.err
}

func makeTx(t *testing.T, nonce uint64, signer crypto.Signer) txn.Transaction {
	opts := []signed.TransactionOption{
		signed.WithArg(native.ContractArg, []byte(testContractName)),
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
		signed.WithArg(native.ContractArg, []byte(viewchange.ContractName)),
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
		m := minoch.MustCreate(manager, fmt.Sprintf("node%d", i))

		addrs[i] = m.GetAddress()

		signer := bls.NewSigner()
		pubkeys[i] = signer.GetPublicKey()

		c := threshold.NewThreshold(m, signer)
		c.SetThreshold(threshold.ByzantineThreshold)

		dir, err := os.MkdirTemp(os.TempDir(), "cosipbft")
		require.NoError(t, err)

		db, err := kv.New(filepath.Join(dir, "test.db"))
		require.NoError(t, err)

		txFac := signed.NewTransactionFactory()

		pool, err := poolimpl.NewPool(gossip.NewFlat(m, txFac))
		require.NoError(t, err)

		tree := binprefix.NewMerkleTree(db, binprefix.Nonce{})

		exec := native.NewExecution()
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
			onet:    m,
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
	return nil, fake.GetError()
}

type badPool struct {
	pool.Pool
}

func (p badPool) SetPlayers(mino.Players) error {
	return fake.GetError()
}

func (p badPool) AddFilter(pool.Filter) {}

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
	return nil, fake.GetError()
}

type fakeValidation struct {
	validation.Service

	err error
}

func (val fakeValidation) Accept(store.Readable, txn.Transaction, validation.Leeway) error {
	return val.err
}

func (val fakeValidation) Validate(store.Snapshot, []txn.Transaction) (validation.Result, error) {
	return simple.NewResult(nil), val.err
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
