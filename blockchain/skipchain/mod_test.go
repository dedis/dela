package skipchain

import (
	"context"
	fmt "fmt"
	"testing"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/blockchain"
	"go.dedis.ch/dela/consensus"
	"go.dedis.ch/dela/consensus/cosipbft"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/cosi/threshold"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/encoding"
	internal "go.dedis.ch/dela/internal/testing"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minoch"
	"golang.org/x/xerrors"
)

func TestMessages(t *testing.T) {
	messages := []proto.Message{
		&BlockProto{},
		&VerifiableBlockProto{},
		&PropagateGenesis{},
		&BlockRequest{},
		&BlockResponse{},
	}

	for _, m := range messages {
		internal.CoverProtoMessage(t, m)
	}
}

func TestSkipchain_Basic(t *testing.T) {
	n := 5

	authority, skipchains, actors := makeSkipchain(t, 3)

	err := actors[0].InitChain(&empty.Empty{}, authority)
	require.NoError(t, err)
	err = actors[1].InitChain(&empty.Empty{}, authority)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	blocks := skipchains[2].Watch(ctx)

	for i := 0; i < n; i++ {
		err = actors[0].Store(&empty.Empty{}, authority)
		require.NoError(t, err)

		event := <-blocks
		require.NotNil(t, event)
		require.IsType(t, SkipBlock{}, event)

		chain, err := skipchains[2].GetVerifiableBlock()
		require.NoError(t, err)

		packed, err := chain.Pack(skipchains[2].encoder)
		require.NoError(t, err)

		block, err := skipchains[2].GetBlockFactory().FromVerifiable(packed)
		require.NoError(t, err)
		require.NotNil(t, block)
		require.Equal(t, uint64(i+1), block.(SkipBlock).Index)
		require.Equal(t, event, block)
	}
}

func TestSkipchain_Listen(t *testing.T) {
	s := &Skipchain{
		mino:      fake.Mino{},
		consensus: fakeConsensus{},
	}

	actor, err := s.Listen(nil)
	require.NoError(t, err)
	require.NotNil(t, actor)

	s.mino = fake.NewBadMino()
	_, err = s.Listen(nil)
	require.EqualError(t, err, "couldn't create the rpc: fake error")

	s.mino = fake.Mino{}
	s.consensus = fakeConsensus{err: xerrors.New("oops")}
	_, err = s.Listen(nil)
	require.EqualError(t, err, "couldn't start the consensus: oops")
}

func TestSkipchain_GetBlock(t *testing.T) {
	digest := Digest{1, 2, 3}
	db := &fakeDatabase{blocks: []SkipBlock{{hash: digest}}}
	s := &Skipchain{db: db}

	block, err := s.GetBlock()
	require.NoError(t, err)
	require.Equal(t, digest, block.(SkipBlock).hash)

	db.err = xerrors.New("oops")
	_, err = s.GetBlock()
	require.EqualError(t, err, "couldn't read the latest block: oops")
}

func TestSkipchain_GetVerifiableBlock(t *testing.T) {
	digest := Digest{1, 2, 3}
	db := &fakeDatabase{blocks: []SkipBlock{{hash: digest}}}
	s := &Skipchain{
		db:        db,
		consensus: fakeConsensus{},
	}

	block, err := s.GetVerifiableBlock()
	require.NoError(t, err)
	require.Equal(t, digest, block.(VerifiableBlock).hash)
	require.NotNil(t, block.(VerifiableBlock).Chain)

	db.err = xerrors.New("oops")
	_, err = s.GetVerifiableBlock()
	require.EqualError(t, err, "couldn't read the latest block: oops")

	db.err = nil
	s.consensus = fakeConsensus{err: xerrors.New("oops")}
	_, err = s.GetVerifiableBlock()
	require.EqualError(t, err, "couldn't read the chain: oops")
}

func TestSkipchain_Watch(t *testing.T) {
	w := &fakeWatcher{}
	s := &Skipchain{
		watcher: w,
	}

	ctx, cancel := context.WithCancel(context.Background())
	blocks := s.Watch(ctx)
	require.NotNil(t, blocks)
	require.Equal(t, 1, w.count)

	cancel()
	_, ok := <-blocks
	require.False(t, ok)
	require.Equal(t, 0, w.count)
}

func TestActor_InitChain(t *testing.T) {
	db := &fakeDatabase{blocks: []SkipBlock{{}}, err: NewNoBlockError(0)}
	actor := skipchainActor{
		operations: &operations{
			encoder: encoding.NewProtoEncoder(),
			blockFactory: blockFactory{
				encoder:     encoding.NewProtoEncoder(),
				hashFactory: crypto.NewSha256Factory(),
			},
			addr: fake.NewAddress(0),
			db:   db,
			rpc:  fakeRPC{},
		},
		rand: crypto.CryptographicRandomGenerator{},
	}

	authority := fake.NewAuthority(3, fake.NewSigner)

	err := actor.InitChain(&empty.Empty{}, authority)
	require.NoError(t, err)

	actor.rpc = fakeRPC{err: xerrors.New("oops")}
	err = actor.InitChain(&empty.Empty{}, authority)
	require.EqualError(t, xerrors.Unwrap(err), "couldn't propagate: oops")

	// No error so the genesis block exists already.
	db.err = nil
	err = actor.InitChain(&empty.Empty{}, authority)
	require.NoError(t, err)

	// Unexpected database error
	db.err = xerrors.New("oops")
	err = actor.InitChain(&empty.Empty{}, authority)
	require.EqualError(t, err, "couldn't read the genesis block: oops")
}

func TestActor_NewChain(t *testing.T) {
	actor := skipchainActor{
		operations: &operations{
			encoder: encoding.NewProtoEncoder(),
			db:      &fakeDatabase{},
			blockFactory: blockFactory{
				encoder:     encoding.NewProtoEncoder(),
				hashFactory: crypto.NewSha256Factory(),
			},
			rpc: fakeRPC{},
		},
		rand: crypto.CryptographicRandomGenerator{},
	}

	authority := fake.NewAuthority(3, fake.NewSigner)

	actor.rand = fakeRandGenerator{err: xerrors.New("oops")}
	err := actor.newChain(&empty.Empty{}, authority)
	require.EqualError(t, err, "couldn't generate backlink: oops")

	actor.rand = fakeRandGenerator{noSize: true}
	err = actor.newChain(&empty.Empty{}, authority)
	require.EqualError(t, err, "mismatch rand length 0 != 32")

	actor.rand = crypto.CryptographicRandomGenerator{}
	actor.encoder = fake.BadPackEncoder{}
	err = actor.newChain(&empty.Empty{}, authority)
	require.EqualError(t, err, "couldn't pack genesis: fake error")

	actor.blockFactory.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	err = actor.newChain(&empty.Empty{}, authority)
	require.Contains(t, err.Error(), "couldn't create block: ")
}

func TestActor_Store(t *testing.T) {
	cons := &fakeConsensusActor{}
	db := &fakeDatabase{blocks: []SkipBlock{{}}}
	actor := skipchainActor{
		operations: &operations{
			encoder: encoding.NewProtoEncoder(),
			blockFactory: blockFactory{
				encoder:     encoding.NewProtoEncoder(),
				hashFactory: crypto.NewSha256Factory(),
			},
			addr: fake.NewAddress(0),
			db:   db,
		},
		consensus: cons,
	}

	authority := fake.NewAuthority(3, fake.NewSigner)

	err := actor.Store(&empty.Empty{}, authority)
	require.NoError(t, err)
	// Make sure the conodes rotate if the view change allows it.
	require.NotNil(t, cons.prop)

	db.err = xerrors.New("oops")
	err = actor.Store(&empty.Empty{}, authority)
	require.EqualError(t, err, "couldn't read the latest block: oops")

	db.err = nil
	actor.blockFactory.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	err = actor.Store(&empty.Empty{}, authority)
	require.Contains(t, err.Error(), "couldn't create next block: ")

	actor.blockFactory.hashFactory = crypto.NewSha256Factory()
	actor.consensus = &fakeConsensusActor{err: xerrors.New("oops")}
	err = actor.Store(&empty.Empty{}, authority)
	require.EqualError(t, err, "couldn't propose the block: oops")
}

func TestObserver_NotifyCallback(t *testing.T) {
	obs := skipchainObserver{
		ch: make(chan blockchain.Block, 1),
	}

	obs.NotifyCallback(struct{}{})
	require.Len(t, obs.ch, 0)

	obs.NotifyCallback(SkipBlock{Index: 1})
	block := <-obs.ch
	require.Equal(t, uint64(1), block.(SkipBlock).Index)
}

// -----------------
// Utility functions

type testValidator struct{}

func (v testValidator) Validate(index uint64, payload proto.Message) error {
	return nil
}

func (v testValidator) Commit(payload proto.Message) error {
	return nil
}

func makeSkipchain(t *testing.T, n int) (crypto.CollectiveAuthority, []*Skipchain, []blockchain.Actor) {
	manager := minoch.NewManager()

	mm := make([]mino.Mino, n)
	for i := 0; i < n; i++ {
		m, err := minoch.NewMinoch(manager, fmt.Sprintf("node%d", i))
		require.NoError(t, err)
		mm[i] = m
	}

	authority := fake.NewAuthorityFromMino(bls.NewSigner, mm...)

	skipchains := make([]*Skipchain, n)
	actors := make([]blockchain.Actor, n)
	for i := 0; i < n; i++ {
		cosi := threshold.NewCoSi(mm[i], authority.GetSigner(i))
		cons := cosipbft.NewCoSiPBFT(mm[i], cosi, fakeGovernance{authority: authority})
		skipchains[i] = NewSkipchain(mm[i], cons)

		actor, err := skipchains[i].Listen(testValidator{})
		require.NoError(t, err)

		actors[i] = actor
	}

	return authority, skipchains, actors
}

type fakeRPC struct {
	mino.RPC
	err error
}

func (rpc fakeRPC) Call(context.Context, proto.Message,
	mino.Players) (<-chan proto.Message, <-chan error) {

	errs := make(chan error, 1)
	if rpc.err != nil {
		errs <- rpc.err
	}
	msgs := make(chan proto.Message)
	if rpc.err == nil {
		close(msgs)
	}
	return msgs, errs
}

type fakeConsensusActor struct {
	consensus.Actor
	err  error
	prop consensus.Proposal
}

func (a *fakeConsensusActor) Propose(prop consensus.Proposal) error {
	a.prop = prop
	return a.err
}

type fakeRandGenerator struct {
	noSize bool
	err    error
}

func (rand fakeRandGenerator) Read(buffer []byte) (int, error) {
	if rand.noSize {
		return 0, nil
	}
	return len(buffer), rand.err
}

type fakeGovernance struct {
	viewchange.Governance
	authority fake.CollectiveAuthority
}

func (gov fakeGovernance) GetAuthority(index uint64) (viewchange.EvolvableAuthority, error) {
	return gov.authority, nil
}

func (gov fakeGovernance) GetChangeSet(consensus.Proposal) (viewchange.ChangeSet, error) {
	return viewchange.ChangeSet{}, nil
}
