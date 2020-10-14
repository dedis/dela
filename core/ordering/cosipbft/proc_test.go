package cosipbft

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/core/ordering/cosipbft/blockstore"
	"go.dedis.ch/dela/core/ordering/cosipbft/blocksync"
	"go.dedis.ch/dela/core/ordering/cosipbft/pbft"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/hashtree"
	"go.dedis.ch/dela/core/validation/simple"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde/json"
)

func TestProcessor_BlockMessage_Invoke(t *testing.T) {
	expected := types.Digest{1}

	proc := newProcessor()
	proc.rosterFac = authority.NewFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})
	proc.sync = fakeSync{latest: 1}
	proc.blocks = fakeStore{}
	proc.pbftsm = fakeSM{
		state: pbft.InitialState,
		id:    expected,
	}

	msg := types.NewBlockMessage(types.Block{}, nil)

	id, err := proc.Invoke(fake.NewAddress(0), msg)
	require.NoError(t, err)
	require.Equal(t, expected[:], id)

	proc.pbftsm = fakeSM{state: pbft.InitialState, err: fake.GetError()}
	_, err = proc.Invoke(fake.NewAddress(0), msg)
	require.EqualError(t, err, fake.Err("pbft prepare failed"))

	views := map[mino.Address]types.ViewMessage{fake.NewAddress(0): {}}
	msg = types.NewBlockMessage(types.Block{}, views)
	proc.pbftsm = fakeSM{err: fake.GetError()}
	_, err = proc.Invoke(fake.NewAddress(0), msg)
	require.EqualError(t, err, fake.Err("accept all"))
}

func TestProcessor_CommitMessage_Invoke(t *testing.T) {
	proc := newProcessor()
	proc.pbftsm = fakeSM{}

	msg := types.NewCommit(types.Digest{1}, fake.Signature{})

	id, err := proc.Invoke(fake.NewAddress(0), msg)
	require.NoError(t, err)
	require.Equal(t, []byte{0xfe}, id)

	proc.pbftsm = fakeSM{err: fake.GetError()}
	_, err = proc.Invoke(fake.NewAddress(0), msg)
	require.EqualError(t, err, fake.Err("pbft commit failed"))

	proc.pbftsm = fakeSM{}
	msg = types.NewCommit(types.Digest{}, fake.NewBadSignature())
	_, err = proc.Invoke(fake.NewAddress(0), msg)
	require.EqualError(t, err, fake.Err("couldn't marshal signature"))

	_, err = proc.Invoke(fake.NewAddress(0), fake.Message{})
	require.EqualError(t, err, "unsupported message of type 'fake.Message'")
}

func TestProcessor_GenesisMessage_Process(t *testing.T) {
	proc := newProcessor()
	proc.tree = blockstore.NewTreeCache(fakeTree{})
	proc.genesis = blockstore.NewGenesisStore()
	proc.access = fakeAccess{}

	root := types.Digest{}
	copy(root[:], []byte("root"))

	ro := authority.FromAuthority(fake.NewAuthority(3, fake.NewSigner))

	genesis, err := types.NewGenesis(ro, types.WithGenesisRoot(root))
	require.NoError(t, err)

	req := mino.Request{
		Message: types.NewGenesisMessage(genesis),
	}

	msg, err := proc.Process(req)
	require.NoError(t, err)
	require.Nil(t, msg)

	proc.genesis = blockstore.NewGenesisStore()
	proc.context = fake.NewContext()
	_, err = proc.Process(req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to serialize roster: couldn't encode roster: ")

	wrongGenesis, err := types.NewGenesis(ro)
	require.NoError(t, err)

	proc.context = json.NewContext()
	_, err = proc.Process(mino.Request{Message: types.NewGenesisMessage(wrongGenesis)})
	require.EqualError(t, err, "mismatch tree root '00000000' != '726f6f74'")

	proc.access = fakeAccess{err: fake.GetError()}
	_, err = proc.Process(req)
	require.EqualError(t, err, fake.Err("while updating tree: failed to set access"))

	proc.access = fakeAccess{}
	proc.tree = blockstore.NewTreeCache(fakeTree{errStore: fake.GetError()})
	_, err = proc.Process(req)
	require.EqualError(t, err, fake.Err("while updating tree: failed to store roster"))

	proc.tree = blockstore.NewTreeCache(fakeTree{errCommit: fake.GetError()})
	_, err = proc.Process(req)
	require.EqualError(t, err, fake.Err("tree commit failed"))

	proc.tree = blockstore.NewTreeCache(fakeTree{})
	proc.genesis = fakeGenesisStore{errSet: fake.GetError()}
	_, err = proc.Process(req)
	require.EqualError(t, err, fake.Err("set genesis failed"))
}

func TestProcessor_DoneMessage_Process(t *testing.T) {
	proc := newProcessor()
	proc.pbftsm = fakeSM{}
	proc.blocks = blockstore.NewInMemory()
	proc.blocks.Store(makeBlock(t, types.Digest{}))

	req := mino.Request{
		Message: types.NewDone(types.Digest{}, fake.Signature{}),
	}

	resp, err := proc.Process(req)
	require.NoError(t, err)
	require.Nil(t, resp)

	proc.pbftsm = fakeSM{err: fake.GetError()}
	_, err = proc.Process(req)
	require.EqualError(t, err, fake.Err("pbftsm finalized failed"))
}

func TestProcessor_ViewMessage_Process(t *testing.T) {
	proc := newProcessor()
	proc.pbftsm = fakeSM{}

	req := mino.Request{
		Message: types.NewViewMessage(types.Digest{}, 0, fake.Signature{}),
	}

	resp, err := proc.Process(req)
	require.NoError(t, err)
	require.Nil(t, resp)

	proc.pbftsm = fakeSM{err: fake.GetError()}
	_, err = proc.Process(req)
	require.NoError(t, err)
}

func TestProcessor_Unsupported_Process(t *testing.T) {
	proc := newProcessor()

	req := mino.Request{Message: fake.Message{}}

	_, err := proc.Process(req)
	require.EqualError(t, err, "unsupported message of type 'fake.Message'")
}

// -----------------------------------------------------------------------------
// Utility functions

func makeBlock(t *testing.T, from types.Digest, opts ...types.LinkOption) types.BlockLink {
	block, err := types.NewBlock(simple.NewResult(nil))
	require.NoError(t, err)

	link, err := types.NewBlockLink(from, block, opts...)
	require.NoError(t, err)

	return link
}

type fakeSM struct {
	pbft.StateMachine

	err       error
	errLeader error
	state     pbft.State
	id        types.Digest
	ch        chan pbft.State
}

func (sm fakeSM) GetState() pbft.State {
	return sm.state
}

func (sm fakeSM) GetLeader() (mino.Address, error) {
	return fake.NewAddress(0), sm.errLeader
}

func (sm fakeSM) GetViews() map[mino.Address]pbft.View {
	return nil
}

func (sm fakeSM) PrePrepare(authority.Authority) error {
	return sm.err
}

func (sm fakeSM) Prepare(mino.Address, types.Block) (types.Digest, error) {
	return sm.id, sm.err
}

func (sm fakeSM) Commit(types.Digest, crypto.Signature) error {
	return sm.err
}

func (sm fakeSM) Finalize(types.Digest, crypto.Signature) error {
	return sm.err
}

func (sm fakeSM) Expire(mino.Address) (pbft.View, error) {
	return pbft.View{}, sm.err
}

func (sm fakeSM) Accept(pbft.View) error {
	return sm.err
}

func (sm fakeSM) AcceptAll([]pbft.View) error {
	return sm.err
}

func (sm fakeSM) Watch(context.Context) <-chan pbft.State {
	return sm.ch
}

type fakeSync struct {
	blocksync.Synchronizer

	latest uint64
	err    error
}

func (sync fakeSync) GetLatest() uint64 {
	return sync.latest
}

func (sync fakeSync) Sync(ctx context.Context, players mino.Players, cfg blocksync.Config) error {
	return sync.err
}

type fakeSnapshot struct {
	store.Snapshot

	err error
}

func (snap fakeSnapshot) Get(key []byte) ([]byte, error) {
	return []byte{}, snap.err
}

func (snap fakeSnapshot) Set(key []byte, value []byte) error {
	return snap.err
}

func (snap fakeSnapshot) Delete(key []byte) error {
	return snap.err
}

type fakeTree struct {
	hashtree.StagingTree

	err       error
	errStage  error
	errCommit error
	errStore  error
}

func (t fakeTree) GetRoot() []byte {
	return []byte("root")
}

func (t fakeTree) GetPath(key []byte) (hashtree.Path, error) {
	return nil, t.err
}

func (t fakeTree) Get(key []byte) ([]byte, error) {
	return []byte("[]"), t.err
}

func (t fakeTree) Stage(fn func(store.Snapshot) error) (hashtree.StagingTree, error) {
	err := fn(fakeSnapshot{err: t.errStore})
	if err != nil {
		return nil, err
	}

	return t, t.errStage
}

func (t fakeTree) Commit() error {
	return t.errCommit
}

type fakeGenesisStore struct {
	blockstore.GenesisStore

	errGet error
	errSet error
}

func (s fakeGenesisStore) Exists() bool {
	return false
}

func (s fakeGenesisStore) Get() (types.Genesis, error) {
	return types.Genesis{}, s.errGet
}

func (s fakeGenesisStore) Set(types.Genesis) error {
	return s.errSet
}

type fakeStore struct {
	blockstore.BlockStore
}

func (fakeStore) Len() uint64 {
	return 0
}

func (fakeStore) Watch(context.Context) <-chan types.BlockLink {
	ch := make(chan types.BlockLink, 1)

	block, _ := types.NewBlock(simple.NewResult(nil), types.WithIndex(1))
	link, _ := types.NewBlockLink(types.Digest{}, block)
	ch <- link
	close(ch)

	return ch
}
