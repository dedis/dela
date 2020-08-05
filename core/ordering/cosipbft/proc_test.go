package cosipbft

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	"go.dedis.ch/dela/core/ordering/cosipbft/blockstore"
	"go.dedis.ch/dela/core/ordering/cosipbft/pbft"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/hashtree"
	"go.dedis.ch/dela/core/validation/simple"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

func TestProcessor_BlockMessage_Invoke(t *testing.T) {
	expected := types.Digest{1}

	proc := newProcessor()
	proc.rosterFac = roster.NewFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})
	proc.pbftsm = fakeSM{
		state: pbft.PrePrepareState,
		id:    expected,
	}

	msg := types.NewBlockMessage(types.Block{})

	id, err := proc.Invoke(fake.NewAddress(0), msg)
	require.NoError(t, err)
	require.Equal(t, expected[:], id)

	proc.pbftsm = fakeSM{state: pbft.InitialState}
	proc.tree = fakeTree{err: xerrors.New("oops")}
	_, err = proc.Invoke(fake.NewAddress(0), msg)
	require.EqualError(t, err, "read roster failed: read from tree: oops")

	proc.tree = fakeTree{}
	proc.pbftsm = fakeSM{state: pbft.InitialState, err: xerrors.New("oops")}
	_, err = proc.Invoke(fake.NewAddress(0), msg)
	require.EqualError(t, err, "pbft pre-prepare failed: oops")

	proc.pbftsm = fakeSM{state: pbft.PrePrepareState, err: xerrors.New("oops")}
	_, err = proc.Invoke(fake.NewAddress(0), msg)
	require.EqualError(t, err, "pbft prepare failed: oops")
}

func TestProcessor_CommitMessage_Invoke(t *testing.T) {
	proc := newProcessor()
	proc.pbftsm = fakeSM{}

	msg := types.NewCommit(types.Digest{1}, fake.Signature{})

	id, err := proc.Invoke(fake.NewAddress(0), msg)
	require.NoError(t, err)
	require.Equal(t, []byte{0xfe}, id)

	proc.pbftsm = fakeSM{err: xerrors.New("oops")}
	_, err = proc.Invoke(fake.NewAddress(0), msg)
	require.EqualError(t, err, "pbft commit failed: oops")

	proc.pbftsm = fakeSM{}
	msg = types.NewCommit(types.Digest{}, fake.NewBadSignature())
	_, err = proc.Invoke(fake.NewAddress(0), msg)
	require.EqualError(t, err, "couldn't marshal signature: fake error")

	_, err = proc.Invoke(fake.NewAddress(0), fake.Message{})
	require.EqualError(t, err, "unsupported message of type 'fake.Message'")
}

func TestProcessor_GenesisMessage_Process(t *testing.T) {
	proc := newProcessor()
	proc.tree = fakeTree{}
	proc.genesis = blockstore.NewGenesisStore()

	genesis, err := types.NewGenesis(fake.NewAuthority(3, fake.NewSigner))
	require.NoError(t, err)

	req := mino.Request{
		Message: types.NewGenesisMessage(genesis),
	}

	msg, err := proc.Process(req)
	require.NoError(t, err)
	require.Nil(t, msg)

	proc.tree = fakeTree{errStage: xerrors.New("oops")}
	_, err = proc.Process(req)
	require.EqualError(t, err, "tree stage failed: oops")

	proc.tree = fakeTree{errCommit: xerrors.New("oops")}
	_, err = proc.Process(req)
	require.EqualError(t, err, "tree commit failed: oops")

	proc.tree = fakeTree{}
	proc.genesis = badGenesisStore{}
	_, err = proc.Process(req)
	require.EqualError(t, err, "set genesis failed: oops")
}

func TestProcessor_DoneMessage_Process(t *testing.T) {
	block, err := types.NewBlock(simple.NewData(nil))
	require.NoError(t, err)

	proc := newProcessor()
	proc.pbftsm = fakeSM{}
	proc.blocks = blockstore.NewInMemory()
	proc.blocks.Store(types.NewBlockLink(types.Digest{}, block))

	req := mino.Request{
		Message: types.NewDone(types.Digest{}, fake.Signature{}),
	}

	resp, err := proc.Process(req)
	require.NoError(t, err)
	require.Nil(t, resp)

	proc.pbftsm = fakeSM{err: xerrors.New("oops")}
	_, err = proc.Process(req)
	require.EqualError(t, err, "pbftsm finalized failed: oops")

	proc.pbftsm = fakeSM{}
	proc.blocks = badBlockStore{}
	_, err = proc.Process(req)
	require.EqualError(t, err, "couldn't get latest block: oops")
}

func TestProcessor_ViewMessage_Process(t *testing.T) {
	proc := newProcessor()
	proc.pbftsm = fakeSM{}

	req := mino.Request{Message: types.NewViewMessage(types.Digest{}, 0)}

	resp, err := proc.Process(req)
	require.NoError(t, err)
	require.Nil(t, resp)
}

func TestProcessor_Unsupported_Process(t *testing.T) {
	proc := newProcessor()

	req := mino.Request{Message: fake.Message{}}

	_, err := proc.Process(req)
	require.EqualError(t, err, "unsupported message of type 'fake.Message'")
}

// Utility functions -----------------------------------------------------------

type fakeSM struct {
	pbft.StateMachine

	err   error
	state pbft.State
	id    types.Digest
	ch    chan pbft.State
}

func (sm fakeSM) GetState() pbft.State {
	return sm.state
}

func (sm fakeSM) GetLeader() mino.Address {
	return fake.NewAddress(0)
}

func (sm fakeSM) PrePrepare(viewchange.Authority) error {
	return sm.err
}

func (sm fakeSM) Prepare(types.Block, hashtree.Tree) (types.Digest, error) {
	return sm.id, sm.err
}

func (sm fakeSM) Commit(types.Digest, crypto.Signature) error {
	return sm.err
}

func (sm fakeSM) Finalize(types.Digest, crypto.Signature) (hashtree.Tree, error) {
	return fakeTree{}, sm.err
}

func (sm fakeSM) Expire(mino.Address) (pbft.View, error) {
	return pbft.View{}, sm.err
}

func (sm fakeSM) Accept(pbft.View) {}

func (sm fakeSM) Watch(context.Context) <-chan pbft.State {
	return sm.ch
}

type fakeSnapshot struct {
	store.Snapshot
}

func (snap fakeSnapshot) Get(key []byte) ([]byte, error) {
	return []byte{}, nil
}

func (snap fakeSnapshot) Set(key []byte, value []byte) error {
	return nil
}

func (snap fakeSnapshot) Delete(key []byte) error {
	return nil
}

type fakeTree struct {
	hashtree.StagingTree

	err       error
	errStage  error
	errCommit error
}

func (t fakeTree) GetRoot() []byte {
	return []byte("root")
}

func (t fakeTree) Get(key []byte) ([]byte, error) {
	return []byte("[]"), t.err
}

func (t fakeTree) Stage(fn func(store.Snapshot) error) (hashtree.StagingTree, error) {
	err := fn(fakeSnapshot{})
	if err != nil {
		return nil, err
	}

	return t, t.errStage
}

func (t fakeTree) Commit() (hashtree.Tree, error) {
	return t, t.errCommit
}

type badGenesisStore struct {
	blockstore.GenesisStore
}

func (s badGenesisStore) Set(types.Genesis) error {
	return xerrors.New("oops")
}

type badBlockStore struct {
	blockstore.BlockStore
}

func (s badBlockStore) Last() (types.BlockLink, error) {
	return types.BlockLink{}, xerrors.New("oops")
}