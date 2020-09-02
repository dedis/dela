package pbft

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/blockchain"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/ordering/cosipbft/blockstore"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/hashtree"
	"go.dedis.ch/dela/core/store/hashtree/binprefix"
	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/core/validation/simple"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/internal/testing/fake"
	"golang.org/x/xerrors"
)

func TestState_String(t *testing.T) {
	var state State = 99
	require.Equal(t, "none", state.String())
}

func TestStateMachine_GetState(t *testing.T) {
	sm := &pbftsm{}

	require.Equal(t, InitialState, sm.GetState())

	sm.state = CommitState
	require.Equal(t, CommitState, sm.GetState())
}

func TestStateMachine_GetLeader(t *testing.T) {
	authority := fake.NewAuthority(3, fake.NewSigner)

	sm := &pbftsm{
		tree: blockstore.NewTreeCache(badTree{}),
		authReader: func(hashtree.Tree) (viewchange.Authority, error) {
			return roster.FromAuthority(authority), nil
		},
	}

	leader, err := sm.GetLeader()
	require.NoError(t, err)
	require.Equal(t, authority.GetAddress(0), leader)

	sm.round.leader = 2
	leader, err = sm.GetLeader()
	require.NoError(t, err)
	require.Equal(t, authority.GetAddress(2), leader)

	sm.authReader = badReader
	_, err = sm.GetLeader()
	require.EqualError(t, err, "failed to read roster: oops")
}

func TestStateMachine_Prepare(t *testing.T) {
	tree, db, clean := makeTree(t)
	defer clean()

	ro := roster.FromAuthority(fake.NewAuthority(3, fake.NewSigner))

	param := StateMachineParam{
		Validation: simple.NewService(fakeExec{}, nil),
		Blocks:     blockstore.NewInMemory(),
		Genesis:    blockstore.NewGenesisStore(),
		Tree:       blockstore.NewTreeCache(tree),
		AuthorityReader: func(hashtree.Tree) (viewchange.Authority, error) {
			return ro, nil
		},
		DB: db,
	}

	param.Genesis.Set(types.Genesis{})

	root := types.Digest{}
	copy(root[:], tree.GetRoot())

	block, err := types.NewBlock(simple.NewData(nil), types.WithTreeRoot(root), types.WithIndex(1))
	require.NoError(t, err)

	sm := NewStateMachine(param).(*pbftsm)
	sm.state = InitialState

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
	require.EqualError(t, err, "mismatch state viewchange != initial")

	sm.state = InitialState
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

	// Failure to read the roster of the round.
	sm.genesis.Set(types.Genesis{})
	sm.authReader = badReader
	_, err = sm.Prepare(block)
	require.EqualError(t, err, "failed to read roster: oops")

	// Failure to read the roster of the staging tree.
	err = sm.verifyPrepare(tree, block, &sm.round, ro)
	require.EqualError(t, err, "failed to read next roster: oops")

	sm.authReader = param.AuthorityReader
	sm.hashFac = fake.NewHashFactory(fake.NewBadHash())
	_, err = sm.Prepare(block)
	require.EqualError(t, err,
		"failed to create link: failed to fingerprint: couldn't write from: fake error")
}

func TestStateMachine_Commit(t *testing.T) {
	sm := &pbftsm{
		state:       PrepareState,
		verifierFac: fake.NewVerifierFactory(fake.Verifier{}),
		watcher:     blockchain.NewWatcher(),
		tree:        blockstore.NewTreeCache(badTree{}),
		authReader: func(hashtree.Tree) (viewchange.Authority, error) {
			return roster.New(nil, nil), nil
		},
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

	sm.verifierFac = fake.NewVerifierFactory(fake.Verifier{})
	sm.authReader = badReader
	err = sm.Commit(types.Digest{1}, fake.Signature{})
	require.EqualError(t, err, "failed to read roster: oops")
}

func TestStateMachine_Finalize(t *testing.T) {
	tree, db, clean := makeTree(t)
	defer clean()

	ro := roster.FromAuthority(fake.NewAuthority(3, fake.NewSigner))

	param := StateMachineParam{
		VerifierFactory: fake.NewVerifierFactory(fake.Verifier{}),
		Blocks:          blockstore.NewInMemory(),
		Genesis:         blockstore.NewGenesisStore(),
		Tree:            blockstore.NewTreeCache(tree),
		AuthorityReader: func(hashtree.Tree) (viewchange.Authority, error) {
			return ro, nil
		},
		DB: db,
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

	sm.blocks = badBlockStore{length: 1}
	err = sm.Finalize(types.Digest{1}, fake.Signature{})
	require.EqualError(t, err, "couldn't get latest digest: oops")

	sm.blocks = blockstore.NewInMemory()
	sm.blocks.Store(makeLink(t))
	sm.round.tree = badTree{}
	err = sm.Finalize(types.Digest{1}, fake.Signature{})
	require.EqualError(t, err, "database failed: commit tree: oops")

	sm.genesis.Set(types.Genesis{})
	sm.round.tree = tree.(hashtree.StagingTree)
	sm.hashFac = fake.NewHashFactory(fake.NewBadHash())
	err = sm.Finalize(types.Digest{1}, fake.Signature{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "database failed: creating link:")

	sm.hashFac = crypto.NewSha256Factory()
	sm.blocks = badBlockStore{}
	err = sm.Finalize(types.Digest{1}, fake.Signature{})
	require.EqualError(t, err, "database failed: store block: oops")

	sm.blocks = blockstore.NewInMemory()
	sm.authReader = badReader
	err = sm.Finalize(types.Digest{1}, fake.Signature{})
	require.EqualError(t, err, "failed to read roster: oops")
}

func TestStateMachine_Accept(t *testing.T) {
	sm := &pbftsm{}

	sm.Accept(View{From: fake.NewAddress(0), Leader: 1})
	require.Len(t, sm.round.views, 1)

	sm.Accept(View{From: fake.NewAddress(1), Leader: 1})
	require.Len(t, sm.round.views, 2)

	// Ignore duplicate.
	sm.Accept(View{From: fake.NewAddress(0), Leader: 1})
	require.Len(t, sm.round.views, 2)

	// Ignore views for a different leader.
	sm.Accept(View{From: fake.NewAddress(2), Leader: 5})
	require.Len(t, sm.round.views, 2)

	// Only accept views for the current round ID.
	buffer := new(bytes.Buffer)
	sm.logger = zerolog.New(buffer)
	sm.Accept(View{From: fake.NewAddress(3), Leader: 1, ID: types.Digest{1}})
	require.Len(t, sm.round.views, 2)
	require.Contains(t, buffer.String(), "received a view for a different block")
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
	require.Equal(t, InitialState, sm.state)

	// Only accept if there are enough views.
	sm.AcceptAll([]View{{Leader: 6}})
	require.Equal(t, 5, sm.round.leader)
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
	tree, db, clean := makeTree(t)
	defer clean()

	ro := roster.FromAuthority(fake.NewAuthority(3, fake.NewSigner))

	param := StateMachineParam{
		Validation:      simple.NewService(fakeExec{}, nil),
		VerifierFactory: fake.VerifierFactory{},
		Blocks:          blockstore.NewInMemory(),
		Genesis:         blockstore.NewGenesisStore(),
		Tree:            blockstore.NewTreeCache(tree),
		AuthorityReader: func(hashtree.Tree) (viewchange.Authority, error) {
			return ro, nil
		},
		DB: db,
	}

	param.Genesis.Set(types.Genesis{})

	root := types.Digest{}
	copy(root[:], tree.GetRoot())

	block, err := types.NewBlock(simple.NewData(nil), types.WithTreeRoot(root), types.WithIndex(1))
	require.NoError(t, err)

	sm := NewStateMachine(param).(*pbftsm)

	opts := []types.LinkOption{
		types.WithSignatures(fake.Signature{}, fake.Signature{}),
		types.WithChangeSet(roster.ChangeSet{}),
	}

	link, err := types.NewBlockLink(types.Digest{}, block, opts...)
	require.NoError(t, err)

	err = sm.CatchUp(link)
	require.NoError(t, err)

	err = sm.CatchUp(link)
	require.EqualError(t, err, "prepare failed: mismatch index 1 != 2")

	sm.authReader = badReader
	err = sm.CatchUp(link)
	require.EqualError(t, err, "failed to read roster: oops")

	sm.authReader = param.AuthorityReader
	sm.blocks = blockstore.NewInMemory()
	sm.verifierFac = fake.NewVerifierFactory(fake.NewBadVerifier())
	err = sm.CatchUp(link)
	require.EqualError(t, err, "commit failed: verifier failed: fake error")

	opts = []types.LinkOption{
		types.WithSignatures(fake.NewBadSignature(), fake.Signature{}),
		types.WithChangeSet(roster.ChangeSet{}),
	}

	link, err = types.NewBlockLink(types.Digest{}, block, opts...)
	require.NoError(t, err)
	sm.verifierFac = fake.VerifierFactory{}
	err = sm.CatchUp(link)
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

// -----------------------------------------------------------------------------
// Utility functions

func makeTree(t *testing.T) (hashtree.Tree, kv.DB, func()) {
	dir, err := ioutil.TempDir(os.TempDir(), "pbft")
	require.NoError(t, err)

	db, err := kv.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)

	tree := binprefix.NewMerkleTree(db, binprefix.Nonce{})
	stage, err := tree.Stage(func(store.Snapshot) error { return nil })
	require.NoError(t, err)

	return stage, db, func() { os.RemoveAll(dir) }
}

func makeLink(t *testing.T) types.BlockLink {
	block, err := types.NewBlock(simple.NewData(nil))
	require.NoError(t, err)

	link, err := types.NewBlockLink(types.Digest{}, block)
	require.NoError(t, err)

	return link
}

type fakeExec struct {
	err error
}

func (e fakeExec) Execute(txn.Transaction, store.Snapshot) (execution.Result, error) {
	return execution.Result{}, e.err
}

type badValidation struct {
	validation.Service
}

func (v badValidation) Validate(store.Snapshot, []txn.Transaction) (validation.Data, error) {
	return nil, xerrors.New("oops")
}

type badBlockStore struct {
	blockstore.BlockStore
	length uint64
}

func (s badBlockStore) WithTx(store.Transaction) blockstore.BlockStore {
	return s
}

func (s badBlockStore) Len() uint64 {
	return s.length
}

func (s badBlockStore) Last() (types.BlockLink, error) {
	return nil, xerrors.New("oops")
}

func (s badBlockStore) Store(types.BlockLink) error {
	return xerrors.New("oops")
}

type badTree struct {
	hashtree.StagingTree
}

func (t badTree) WithTx(store.Transaction) hashtree.StagingTree {
	return t
}

func (t badTree) Commit() error {
	return xerrors.New("oops")
}

func badReader(hashtree.Tree) (viewchange.Authority, error) {
	return nil, xerrors.New("oops")
}
