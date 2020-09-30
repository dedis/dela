package pbft

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
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
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
)

func TestState_String(t *testing.T) {
	var state State = 99
	require.Equal(t, "unknown", state.String())

	state = 0
	require.Equal(t, "none", state.String())
}

func TestStateMachine_GetState(t *testing.T) {
	sm := &pbftsm{}
	require.Equal(t, NoneState, sm.GetState())

	sm.state = CommitState
	require.Equal(t, CommitState, sm.GetState())
}

func TestStateMachine_GetLeader(t *testing.T) {
	roster := fake.NewAuthority(3, fake.NewSigner)

	sm := &pbftsm{
		tree: blockstore.NewTreeCache(badTree{}),
		authReader: func(hashtree.Tree) (authority.Authority, error) {
			return authority.FromAuthority(roster), nil
		},
	}

	leader, err := sm.GetLeader()
	require.NoError(t, err)
	require.Equal(t, roster.GetAddress(0), leader)

	sm.round.leader = 2
	leader, err = sm.GetLeader()
	require.NoError(t, err)
	require.Equal(t, roster.GetAddress(2), leader)

	sm.authReader = badReader
	_, err = sm.GetLeader()
	require.EqualError(t, err, fake.Err("failed to read roster"))
}

func TestStateMachine_GetViews(t *testing.T) {
	sm := &pbftsm{}
	require.Len(t, sm.GetViews(), 0)

	sm.round.prevViews = map[mino.Address]View{
		fake.NewAddress(0): {},
		fake.NewAddress(1): {},
	}
	require.Len(t, sm.GetViews(), 2)
}

func TestStateMachine_Prepare(t *testing.T) {
	tree, db, clean := makeTree(t)
	defer clean()

	ro := authority.FromAuthority(fake.NewAuthority(3, fake.NewSigner))

	param := StateMachineParam{
		Validation: simple.NewService(fakeExec{}, nil),
		Blocks:     blockstore.NewInMemory(),
		Genesis:    blockstore.NewGenesisStore(),
		Tree:       blockstore.NewTreeCache(tree),
		AuthorityReader: func(hashtree.Tree) (authority.Authority, error) {
			return ro, nil
		},
		DB: db,
	}

	param.Genesis.Set(types.Genesis{})

	root := types.Digest{}
	copy(root[:], tree.GetRoot())

	block, err := types.NewBlock(simple.NewData(nil), types.WithTreeRoot(root), types.WithIndex(0))
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
	require.EqualError(t, err, fake.Err("tree failed: callback failed: validation failed"))

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
	require.EqualError(t, err, fake.Err("failed to read roster"))

	// Failure to read the roster of the staging tree.
	err = sm.verifyPrepare(tree, block, &sm.round, ro)
	require.EqualError(t, err, fake.Err("failed to read next roster"))

	sm.authReader = param.AuthorityReader
	sm.hashFac = fake.NewHashFactory(fake.NewBadHash())
	_, err = sm.Prepare(block)
	require.EqualError(t, err,
		fake.Err("failed to create link: failed to fingerprint: couldn't write from"))
}

func TestStateMachine_Commit(t *testing.T) {
	sm := &pbftsm{
		state:       PrepareState,
		verifierFac: fake.NewVerifierFactory(fake.Verifier{}),
		watcher:     core.NewWatcher(),
		tree:        blockstore.NewTreeCache(badTree{}),
		authReader: func(hashtree.Tree) (authority.Authority, error) {
			return authority.New(nil, nil), nil
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
	require.EqualError(t, err, fake.Err("couldn't make verifier"))

	sm.verifierFac = fake.NewVerifierFactory(fake.NewBadVerifier())
	err = sm.Commit(types.Digest{1}, fake.Signature{})
	require.EqualError(t, err, fake.Err("verifier failed"))

	sm.verifierFac = fake.NewVerifierFactory(fake.Verifier{})
	sm.authReader = badReader
	err = sm.Commit(types.Digest{1}, fake.Signature{})
	require.EqualError(t, err, fake.Err("failed to read roster"))
}

func TestStateMachine_Finalize(t *testing.T) {
	tree, db, clean := makeTree(t)
	defer clean()

	ro := authority.FromAuthority(fake.NewAuthority(3, fake.NewSigner))

	param := StateMachineParam{
		VerifierFactory: fake.NewVerifierFactory(fake.Verifier{}),
		Blocks:          blockstore.NewInMemory(),
		Genesis:         blockstore.NewGenesisStore(),
		Tree:            blockstore.NewTreeCache(tree),
		AuthorityReader: func(hashtree.Tree) (authority.Authority, error) {
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
	require.EqualError(t, err, fake.Err("couldn't make verifier"))

	sm.verifierFac = fake.NewVerifierFactory(fake.NewBadVerifier())
	sm.round.prepareSig = fake.Signature{}
	err = sm.Finalize(types.Digest{1}, fake.Signature{})
	require.EqualError(t, err, fake.Err("verifier failed"))

	sm.verifierFac = fake.NewVerifierFactory(fake.Verifier{})
	sm.genesis = blockstore.NewGenesisStore()
	sm.blocks = blockstore.NewInMemory()
	err = sm.Finalize(types.Digest{1}, fake.Signature{})
	require.EqualError(t, err, "couldn't get latest digest: missing genesis block")

	sm.blocks = badBlockStore{length: 1}
	err = sm.Finalize(types.Digest{1}, fake.Signature{})
	require.EqualError(t, err, fake.Err("couldn't get latest digest"))

	sm.blocks = blockstore.NewInMemory()
	sm.blocks.Store(makeLink(t))
	sm.round.tree = badTree{}
	err = sm.Finalize(types.Digest{1}, fake.Signature{})
	require.EqualError(t, err, fake.Err("database failed: commit tree"))

	sm.genesis.Set(types.Genesis{})
	sm.round.tree = tree.(hashtree.StagingTree)
	sm.hashFac = fake.NewHashFactory(fake.NewBadHash())
	err = sm.Finalize(types.Digest{1}, fake.Signature{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "database failed: creating link:")

	sm.hashFac = crypto.NewSha256Factory()
	sm.blocks = badBlockStore{}
	err = sm.Finalize(types.Digest{1}, fake.Signature{})
	require.EqualError(t, err, fake.Err("database failed: store block"))

	sm.blocks = blockstore.NewInMemory()
	sm.authReader = badReader
	err = sm.Finalize(types.Digest{1}, fake.Signature{})
	require.EqualError(t, err, fake.Err("failed to read roster"))
}

func TestStateMachine_Accept(t *testing.T) {
	ro := authority.FromAuthority(fake.NewAuthority(4, fake.NewSigner))

	sm := &pbftsm{
		blocks:  blockstore.NewInMemory(),
		genesis: blockstore.NewGenesisStore(),
		watcher: core.NewWatcher(),
		signer:  fake.NewSigner(),
		tree:    blockstore.NewTreeCache(badTree{}),
		authReader: func(hashtree.Tree) (authority.Authority, error) {
			return ro, nil
		},
	}

	sm.genesis.Set(types.Genesis{})

	err := sm.Accept(View{from: fake.NewAddress(0), leader: 1})
	require.NoError(t, err)
	require.Equal(t, 2, sm.round.threshold)
	require.Len(t, sm.round.views, 1)

	err = sm.Accept(View{from: fake.NewAddress(1), leader: 1})
	require.NoError(t, err)
	require.Len(t, sm.round.views, 2)

	// Ignore view for the same leader.
	err = sm.Accept(View{from: fake.NewAddress(2), leader: 0})
	require.NoError(t, err)
	require.Len(t, sm.round.views, 2)

	// Ignore duplicate.
	err = sm.Accept(View{from: fake.NewAddress(0), leader: 1})
	require.NoError(t, err)
	require.Len(t, sm.round.views, 2)

	// Ignore views for a different leader than the next one.
	err = sm.Accept(View{from: fake.NewAddress(2), leader: 5})
	require.EqualError(t, err, "invalid view: mismatch leader 5 != 1")
	require.Len(t, sm.round.views, 2)

	sm.genesis = blockstore.NewGenesisStore()
	err = sm.Accept(View{from: fake.NewAddress(0), leader: 1})
	require.EqualError(t, err, "invalid view: failed to read latest id: missing genesis block")

	// Only accept views for the current round ID.
	sm.genesis.Set(types.Genesis{})
	err = sm.Accept(View{from: fake.NewAddress(3), leader: 1, id: types.Digest{1}})
	require.EqualError(t, err, "invalid view: mismatch id 01000000 != 00000000")

	sm.authReader = badReader
	err = sm.Accept(View{leader: 1})
	require.EqualError(t, err, fake.Err("invalid view: failed to read roster"))

	sm.state = NoneState
	err = sm.Accept(View{leader: 1})
	require.EqualError(t, err, fake.Err("init: failed to read roster"))

	// Ignore view with an invalid signature.
	sm.state = InitialState
	sm.authReader = func(hashtree.Tree) (authority.Authority, error) {
		ro := authority.New(
			[]mino.Address{fake.NewAddress(0)},
			[]crypto.PublicKey{fake.NewBadPublicKey()},
		)
		return ro, nil
	}
	err = sm.Accept(View{from: fake.NewAddress(0), leader: 1})
	require.EqualError(t, err, fake.Err("invalid view: invalid signature: verify"))
}

func TestStateMachine_AcceptAll(t *testing.T) {
	ro := authority.FromAuthority(fake.NewAuthority(4, fake.NewSigner))

	sm := &pbftsm{
		blocks:  blockstore.NewInMemory(),
		genesis: blockstore.NewGenesisStore(),
		watcher: core.NewWatcher(),
		signer:  fake.NewSigner(),
		tree:    blockstore.NewTreeCache(badTree{}),
		authReader: func(hashtree.Tree) (authority.Authority, error) {
			return ro, nil
		},
	}

	sm.genesis.Set(types.Genesis{})

	err := sm.AcceptAll([]View{
		{from: fake.NewAddress(0), leader: 5},
		{from: fake.NewAddress(1), leader: 5},
		{from: fake.NewAddress(2), leader: 5},
	})
	require.NoError(t, err)
	require.Equal(t, 2, sm.round.threshold)
	require.Equal(t, uint16(5), sm.round.leader)
	require.Equal(t, InitialState, sm.state)
	require.Nil(t, sm.round.views)
	require.Len(t, sm.round.prevViews, 3)

	sm.round.threshold = 0
	err = sm.AcceptAll([]View{{leader: 5}})
	require.NoError(t, err)

	// Only accept if there are enough views.
	err = sm.AcceptAll([]View{})
	require.EqualError(t, err, "not enough views: 0 <= 0")

	err = sm.AcceptAll([]View{{from: fake.NewAddress(4), leader: 6}})
	require.EqualError(t, err, "invalid view: unknown peer: fake.Address[4]")

	sm.state = NoneState
	sm.authReader = badReader
	err = sm.AcceptAll([]View{{}})
	require.EqualError(t, err, fake.Err("init: failed to read roster"))
}

func TestStateMachine_Expire(t *testing.T) {
	ro := authority.FromAuthority(fake.NewAuthority(4, fake.NewSigner))

	sm := &pbftsm{
		watcher: core.NewWatcher(),
		blocks:  blockstore.NewInMemory(),
		genesis: blockstore.NewGenesisStore(),
		signer:  bls.NewSigner(),
		tree:    blockstore.NewTreeCache(badTree{}),
		authReader: func(hashtree.Tree) (authority.Authority, error) {
			return ro, nil
		},
	}

	sm.genesis.Set(types.Genesis{})

	view, err := sm.Expire(fake.NewAddress(0))
	require.NoError(t, err)
	require.Equal(t, 2, sm.round.threshold)
	require.Equal(t, uint16(1), view.leader)
	require.NoError(t, view.Verify(sm.signer.GetPublicKey()))

	sm.signer = fake.NewBadSigner()
	_, err = sm.Expire(fake.NewAddress(0))
	require.EqualError(t, err, fake.Err("create view: signer"))

	sm.signer = fake.NewSigner()
	sm.genesis = blockstore.NewGenesisStore()
	_, err = sm.Expire(fake.NewAddress(0))
	require.EqualError(t, err, "couldn't get latest digest: missing genesis block")

	sm.authReader = badReader
	sm.state = NoneState
	_, err = sm.Expire(fake.NewAddress(0))
	require.EqualError(t, err, fake.Err("init: failed to read roster"))
}

func TestStateMachine_CatchUp(t *testing.T) {
	tree, db, clean := makeTree(t)
	defer clean()

	ro := authority.FromAuthority(fake.NewAuthority(3, fake.NewSigner))

	param := StateMachineParam{
		Validation:      simple.NewService(fakeExec{}, nil),
		VerifierFactory: fake.VerifierFactory{},
		Blocks:          blockstore.NewInMemory(),
		Genesis:         blockstore.NewGenesisStore(),
		Tree:            blockstore.NewTreeCache(tree),
		AuthorityReader: func(hashtree.Tree) (authority.Authority, error) {
			return ro, nil
		},
		DB: db,
	}

	param.Genesis.Set(types.Genesis{})

	root := types.Digest{}
	copy(root[:], tree.GetRoot())

	block, err := types.NewBlock(simple.NewData(nil), types.WithTreeRoot(root), types.WithIndex(0))
	require.NoError(t, err)

	sm := NewStateMachine(param).(*pbftsm)

	opts := []types.LinkOption{
		types.WithSignatures(fake.Signature{}, fake.Signature{}),
		types.WithChangeSet(authority.NewChangeSet()),
	}

	link, err := types.NewBlockLink(types.Digest{}, block, opts...)
	require.NoError(t, err)

	err = sm.CatchUp(link)
	require.NoError(t, err)

	err = sm.CatchUp(link)
	require.EqualError(t, err, "prepare failed: mismatch index 0 != 1")

	sm.authReader = badReader
	err = sm.CatchUp(link)
	require.EqualError(t, err, fake.Err("failed to read roster"))

	sm.authReader = param.AuthorityReader
	sm.blocks = blockstore.NewInMemory()
	sm.verifierFac = fake.NewVerifierFactory(fake.NewBadVerifier())
	err = sm.CatchUp(link)
	require.EqualError(t, err, fake.Err("commit failed: verifier failed"))

	opts = []types.LinkOption{
		types.WithSignatures(fake.NewBadSignature(), fake.Signature{}),
		types.WithChangeSet(authority.NewChangeSet()),
	}

	link, err = types.NewBlockLink(types.Digest{}, block, opts...)
	require.NoError(t, err)
	sm.verifierFac = fake.VerifierFactory{}
	err = sm.CatchUp(link)
	require.EqualError(t, err, fake.Err("finalize failed: couldn't marshal signature"))
}

func TestStateMachine_Watch(t *testing.T) {
	sm := &pbftsm{
		watcher: core.NewWatcher(),
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

func (e fakeExec) Execute(store.Snapshot, execution.Step) (execution.Result, error) {
	return execution.Result{}, e.err
}

type badValidation struct {
	validation.Service
}

func (v badValidation) Validate(store.Snapshot, []txn.Transaction) (validation.Data, error) {
	return nil, fake.GetError()
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
	return nil, fake.GetError()
}

func (s badBlockStore) Store(types.BlockLink) error {
	return fake.GetError()
}

type badTree struct {
	hashtree.StagingTree
}

func (t badTree) WithTx(store.Transaction) hashtree.StagingTree {
	return t
}

func (t badTree) Commit() error {
	return fake.GetError()
}

func badReader(hashtree.Tree) (authority.Authority, error) {
	return nil, fake.GetError()
}
