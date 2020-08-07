package pbft

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/blockchain"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/ordering/cosipbft/blockstore"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/hashtree"
	"go.dedis.ch/dela/core/store/hashtree/binprefix"
	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/core/tap"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/core/validation/simple"
	"go.dedis.ch/dela/internal/testing/fake"
	"golang.org/x/xerrors"
)

func TestStateMachine_GetState(t *testing.T) {
	sm := &pbftsm{}

	require.Equal(t, InitialState, sm.GetState())

	sm.state = CommitState
	require.Equal(t, CommitState, sm.GetState())
}

func TestStateMachine_GetLeader(t *testing.T) {
	sm := &pbftsm{}

	leader := sm.GetLeader()
	require.Nil(t, leader)

	authority := fake.NewAuthority(3, fake.NewSigner)

	sm.round.roster = roster.FromAuthority(authority)
	leader = sm.GetLeader()
	require.Equal(t, authority.GetAddress(0), leader)

	sm.round.leader = 2
	leader = sm.GetLeader()
	require.Equal(t, authority.GetAddress(2), leader)
}

func TestStateMachine_PrePrepare(t *testing.T) {
	sm := &pbftsm{
		watcher: blockchain.NewWatcher(),
	}

	ro := roster.FromAuthority(fake.NewAuthority(4, fake.NewSigner))

	err := sm.PrePrepare(ro)
	require.NoError(t, err)
	require.Equal(t, 2, sm.round.threshold)
	require.Equal(t, ro, sm.round.roster)

	err = sm.PrePrepare(ro)
	require.EqualError(t, err, "mismatch state preprepare != initial")
}

func TestStateMachine_Prepare(t *testing.T) {
	tree, clean := makeTree(t)
	defer clean()

	param := StateMachineParam{
		Validation: simple.NewService(fakeExec{}, nil),
		Blocks:     blockstore.NewInMemory(),
		Genesis:    blockstore.NewGenesisStore(),
		Tree:       blockstore.NewTreeCache(tree),
	}

	param.Genesis.Set(types.Genesis{})

	root := types.Digest{}
	copy(root[:], tree.GetRoot())

	block, err := types.NewBlock(simple.NewData(nil), types.WithTreeRoot(root), types.WithIndex(1))
	require.NoError(t, err)

	sm := NewStateMachine(param).(*pbftsm)
	sm.state = PrePrepareState

	id, err := sm.Prepare(block)
	require.NoError(t, err)
	require.NotEqual(t, types.Digest{}, id)
	require.Equal(t, PrepareState, sm.state)
	require.Equal(t, id, sm.round.id)

	id, err = sm.Prepare(block)
	require.NoError(t, err)
	require.Equal(t, sm.round.id, id)

	sm.state = ViewChangeState
	_, err = sm.Prepare(block)
	require.EqualError(t, err, "mismatch state viewchange != preprepare")

	sm.state = PrePrepareState
	sm.val = badValidation{}
	_, err = sm.Prepare(block)
	require.EqualError(t, err, "tree failed: callback failed: validation failed: oops")

	other, err := types.NewBlock(simple.NewData(nil), types.WithTreeRoot(types.Digest{}))
	require.NoError(t, err)

	sm.val = simple.NewService(fakeExec{}, nil)
	_, err = sm.Prepare(other)
	require.EqualError(t, err, "mismatch tree root '71b6c1d5' != '00000000'")

	sm.genesis = blockstore.NewGenesisStore()
	_, err = sm.Prepare(block)
	require.EqualError(t, err, "couldn't get latest digest: missing genesis block")

	sm.genesis.Set(types.Genesis{})
	sm.hashFac = fake.NewHashFactory(fake.NewBadHash())
	_, err = sm.Prepare(block)
	require.EqualError(t, err, "couldn't fingerprint link: couldn't write from: fake error")
}

func TestStateMachine_Commit(t *testing.T) {
	sm := &pbftsm{
		state:       PrepareState,
		verifierFac: fake.NewVerifierFactory(fake.Verifier{}),
		watcher:     blockchain.NewWatcher(),
	}
	sm.round.id = types.Digest{1}

	err := sm.Commit(types.Digest{1}, fake.Signature{})
	require.NoError(t, err)

	err = sm.Commit(types.Digest{}, fake.Signature{})
	require.NoError(t, err)

	sm.state = ViewChangeState
	err = sm.Commit(types.Digest{}, fake.Signature{})
	require.EqualError(t, err, "mismatch state viewchange != prepare")

	sm.state = PrepareState
	err = sm.Commit(types.Digest{2}, fake.Signature{})
	require.EqualError(t, err, "mismatch id '02000000' != '01000000'")

	sm.verifierFac = fake.NewBadVerifierFactory()
	err = sm.Commit(types.Digest{1}, fake.Signature{})
	require.EqualError(t, err, "couldn't make verifier: fake error")

	sm.verifierFac = fake.NewVerifierFactory(fake.NewBadVerifier())
	err = sm.Commit(types.Digest{1}, fake.Signature{})
	require.EqualError(t, err, "verifier failed: fake error")
}

func TestStateMachine_Finalize(t *testing.T) {
	tree, clean := makeTree(t)
	defer clean()

	param := StateMachineParam{
		VerifierFactory: fake.NewVerifierFactory(fake.Verifier{}),
		Blocks:          blockstore.NewInMemory(),
		Genesis:         blockstore.NewGenesisStore(),
		Tree:            blockstore.NewTreeCache(tree),
	}

	param.Genesis.Set(types.Genesis{})

	sm := NewStateMachine(param).(*pbftsm)
	sm.state = CommitState
	sm.round.tree = tree.(hashtree.StagingTree)
	sm.round.prepareSig = fake.Signature{}

	err := sm.Finalize(types.Digest{1}, fake.Signature{})
	require.NoError(t, err)

	err = sm.Finalize(types.Digest{1}, fake.Signature{})
	require.EqualError(t, err, "mismatch state initial != commit")

	sm.state = CommitState
	sm.verifierFac = fake.NewBadVerifierFactory()
	err = sm.Finalize(types.Digest{1}, fake.Signature{})
	require.EqualError(t, err, "couldn't make verifier: fake error")

	sm.verifierFac = fake.NewVerifierFactory(fake.NewBadVerifier())
	sm.round.prepareSig = fake.Signature{}
	err = sm.Finalize(types.Digest{1}, fake.Signature{})
	require.EqualError(t, err, "verifier failed: fake error")

	sm.verifierFac = fake.NewVerifierFactory(fake.Verifier{})
	sm.genesis = blockstore.NewGenesisStore()
	sm.blocks = blockstore.NewInMemory()
	err = sm.Finalize(types.Digest{1}, fake.Signature{})
	require.EqualError(t, err, "couldn't get latest digest: missing genesis block")
}

func TestStateMachine_Accept(t *testing.T) {
	sm := &pbftsm{}

	sm.Accept(View{From: fake.NewAddress(0), Leader: 1})
	require.Len(t, sm.round.views, 1)

	sm.Accept(View{From: fake.NewAddress(1), Leader: 1})
	require.Len(t, sm.round.views, 2)

	sm.Accept(View{From: fake.NewAddress(0), Leader: 1})
	require.Len(t, sm.round.views, 2)

	sm.Accept(View{From: fake.NewAddress(2), Leader: 5})
	require.Len(t, sm.round.views, 2)
}

func TestStateMachine_AcceptAll(t *testing.T) {
	sm := &pbftsm{
		round:   round{threshold: 2},
		watcher: blockchain.NewWatcher(),
	}

	sm.AcceptAll([]View{
		{From: fake.NewAddress(0), Leader: 5},
		{From: fake.NewAddress(1), Leader: 5},
		{From: fake.NewAddress(2), Leader: 5},
	})
	require.Equal(t, 5, sm.round.leader)
	require.Equal(t, PrePrepareState, sm.state)
}

func TestStateMachine_Expire(t *testing.T) {
	sm := &pbftsm{
		watcher: blockchain.NewWatcher(),
		blocks:  blockstore.NewInMemory(),
		genesis: blockstore.NewGenesisStore(),
	}

	sm.genesis.Set(types.Genesis{})

	view, err := sm.Expire(fake.NewAddress(0))
	require.NoError(t, err)
	require.Equal(t, 1, view.Leader)

	sm.genesis = blockstore.NewGenesisStore()
	_, err = sm.Expire(fake.NewAddress(0))
	require.EqualError(t, err, "couldn't get latest digest: missing genesis block")
}

func TestStateMachine_CatchUp(t *testing.T) {
	tree, clean := makeTree(t)
	defer clean()

	param := StateMachineParam{
		Validation:      simple.NewService(fakeExec{}, nil),
		VerifierFactory: fake.VerifierFactory{},
		Blocks:          blockstore.NewInMemory(),
		Genesis:         blockstore.NewGenesisStore(),
		Tree:            blockstore.NewTreeCache(tree),
	}

	param.Genesis.Set(types.Genesis{})

	root := types.Digest{}
	copy(root[:], tree.GetRoot())

	block, err := types.NewBlock(simple.NewData(nil), types.WithTreeRoot(root), types.WithIndex(1))
	require.NoError(t, err)

	sm := NewStateMachine(param).(*pbftsm)

	err = sm.CatchUp(types.NewBlockLink(types.Digest{}, block, fake.Signature{}, fake.Signature{}))
	require.NoError(t, err)

	err = sm.CatchUp(types.NewBlockLink(types.Digest{}, block, fake.Signature{}, fake.Signature{}))
	require.EqualError(t, err, "prepare failed: mismatch index 1 != 2")

	sm.blocks = blockstore.NewInMemory()
	sm.verifierFac = fake.NewVerifierFactory(fake.NewBadVerifier())
	err = sm.CatchUp(types.NewBlockLink(types.Digest{}, block, fake.Signature{}, fake.Signature{}))
	require.EqualError(t, err, "commit failed: verifier failed: fake error")

	sm.verifierFac = fake.VerifierFactory{}
	err = sm.CatchUp(types.NewBlockLink(types.Digest{}, block, fake.NewBadSignature(), fake.Signature{}))
	require.EqualError(t, err, "finalize failed: couldn't marshal signature: fake error")
}

func TestStateMachine_Watch(t *testing.T) {
	sm := &pbftsm{
		watcher: blockchain.NewWatcher(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	statesCh := sm.Watch(ctx)

	sm.setState(ViewChangeState)
	state := <-statesCh
	require.Equal(t, ViewChangeState, state)

	cancel()
	_, more := <-statesCh
	require.False(t, more)
}

// Utility functions -----------------------------------------------------------

func makeTree(t *testing.T) (hashtree.Tree, func()) {
	dir, err := ioutil.TempDir(os.TempDir(), "pbft")
	require.NoError(t, err)

	db, err := kv.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)

	tree := binprefix.NewMerkleTree(db, binprefix.Nonce{})
	stage, err := tree.Stage(func(store.Snapshot) error { return nil })
	require.NoError(t, err)

	return stage, func() { os.RemoveAll(dir) }
}

type fakeExec struct {
	err error
}

func (e fakeExec) Execute(tap.Transaction, store.Snapshot) (execution.Result, error) {
	return execution.Result{}, e.err
}

type badValidation struct {
	validation.Service
}

func (v badValidation) Validate(store.Snapshot, []tap.Transaction) (validation.Data, error) {
	return nil, xerrors.New("oops")
}
