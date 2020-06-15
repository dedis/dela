package skipchain

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/blockchain"
	"go.dedis.ch/dela/consensus"
	"go.dedis.ch/dela/consensus/cosipbft"
	"go.dedis.ch/dela/consensus/viewchange/constant"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	"go.dedis.ch/dela/cosi/flatcosi"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minoch"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
	"golang.org/x/xerrors"
)

func TestSkipchain_Basic(t *testing.T) {
	n := 5

	authority, skipchains, actors := makeSkipchain(t, 3)

	err := actors[0].Setup(fake.Message{}, authority)
	require.NoError(t, err)
	err = actors[1].Setup(fake.Message{}, authority)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	blocks := skipchains[2].Watch(ctx)

	for i := 0; i < n; i++ {
		err = actors[0].Store(fake.Message{}, authority)
		require.NoError(t, err)

		event := <-blocks
		require.NotNil(t, event)
		require.IsType(t, SkipBlock{}, event)

		chain, err := skipchains[2].GetVerifiableBlock()
		require.NoError(t, err)

		ser := json.NewSerializer()

		data, err := ser.Serialize(chain)
		require.NoError(t, err)

		factory := NewVerifiableFactory(NewBlockFactory(fake.MessageFactory{}),
			skipchains[0].consensus.GetChainFactory())

		var block VerifiableBlock
		err = ser.Deserialize(data, factory, &block)
		require.NoError(t, err)
		require.NotNil(t, block)
		require.Equal(t, uint64(i+1), block.Index)
		require.Equal(t, event, block.SkipBlock)
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

func TestActor_Setup(t *testing.T) {
	db := &fakeDatabase{blocks: []SkipBlock{{}}, err: NewNoBlockError(0)}
	actor := skipchainActor{
		operations: &operations{
			blockFactory: BlockFactory{
				hashFactory: crypto.NewSha256Factory(),
			},
			addr: fake.NewAddress(0),
			db:   db,
			rpc:  fakeRPC{},
		},
		rand: crypto.CryptographicRandomGenerator{},
	}

	authority := fake.NewAuthority(3, fake.NewSigner)

	err := actor.Setup(fake.Message{}, authority)
	require.NoError(t, err)

	actor.rpc = fakeRPC{err: xerrors.New("oops")}
	err = actor.Setup(fake.Message{}, authority)
	require.EqualError(t, xerrors.Unwrap(err), "couldn't propagate: oops")

	// No error so the genesis block exists already.
	db.err = nil
	err = actor.Setup(fake.Message{}, authority)
	require.NoError(t, err)

	// Unexpected database error
	db.err = xerrors.New("oops")
	err = actor.Setup(fake.Message{}, authority)
	require.EqualError(t, err, "couldn't read the genesis block: oops")
}

func TestActor_NewChain(t *testing.T) {
	actor := skipchainActor{
		operations: &operations{
			addr: fake.NewAddress(0),
			db:   &fakeDatabase{},
			blockFactory: BlockFactory{
				hashFactory: crypto.NewSha256Factory(),
			},
			rpc: fakeRPC{},
		},
		rand: crypto.CryptographicRandomGenerator{},
	}

	authority := fake.NewAuthority(3, fake.NewSigner)

	actor.rand = fakeRandGenerator{err: xerrors.New("oops")}
	err := actor.newChain(fake.Message{}, authority)
	require.EqualError(t, err, "couldn't generate backlink: oops")

	actor.rand = fakeRandGenerator{noSize: true}
	err = actor.newChain(fake.Message{}, authority)
	require.EqualError(t, err, "mismatch rand length 0 != 32")

	actor.rand = crypto.CryptographicRandomGenerator{}
	actor.blockFactory.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	err = actor.newChain(fake.Message{}, authority)
	require.Contains(t, err.Error(), "couldn't create block: ")
}

func TestActor_Store(t *testing.T) {
	cons := &fakeConsensusActor{}
	db := &fakeDatabase{blocks: []SkipBlock{{}}}
	actor := skipchainActor{
		operations: &operations{
			blockFactory: BlockFactory{
				hashFactory: crypto.NewSha256Factory(),
			},
			addr: fake.NewAddress(0),
			db:   db,
		},
		consensus: cons,
	}

	authority := fake.NewAuthority(3, fake.NewSigner)

	err := actor.Store(fake.Message{}, authority)
	require.NoError(t, err)
	// Make sure the conodes rotate if the view change allows it.
	require.NotNil(t, cons.prop)

	db.err = xerrors.New("oops")
	err = actor.Store(fake.Message{}, authority)
	require.EqualError(t, err, "couldn't read the latest block: oops")

	db.err = nil
	actor.consensus = &fakeConsensusActor{err: xerrors.New("oops")}
	err = actor.Store(fake.Message{}, authority)
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

type testReactor struct {
	fake.MessageFactory
}

func (v testReactor) InvokeValidate(serde.Message) (blockchain.Payload, error) {
	return fake.Message{}, nil
}

func (v testReactor) InvokeCommit(blockchain.Payload) error {
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
	for i, m := range mm {
		cosi := flatcosi.NewFlat(mm[i], authority.GetSigner(i))
		vc := constant.NewViewChange(m.GetAddress(), authority,
			roster.NewChangeSetFactory(m.GetAddressFactory(), cosi.GetPublicKeyFactory()))

		cons := cosipbft.NewCoSiPBFT(mm[i], cosi, vc)
		skipchains[i] = NewSkipchain(mm[i], cons)

		actor, err := skipchains[i].Listen(testReactor{})
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
	prop serde.Message
}

func (a *fakeConsensusActor) Propose(prop serde.Message) error {
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
