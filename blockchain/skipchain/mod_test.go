package skipchain

import (
	"bytes"
	"context"
	"testing"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/cosi/flatcosi"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/crypto/bls"
	"go.dedis.ch/fabric/encoding"
	internal "go.dedis.ch/fabric/internal/testing"
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/fabric/mino/minoch"
	"golang.org/x/xerrors"
)

func TestMessages(t *testing.T) {
	messages := []proto.Message{
		&ConodeProto{},
		&BlockProto{},
		&VerifiableBlockProto{},
		&PropagateGenesis{},
	}

	for _, m := range messages {
		internal.CoverProtoMessage(t, m)
	}
}

func TestSkipchain_Basic(t *testing.T) {
	n := 5
	manager := minoch.NewManager()

	c1, _, a1 := makeSkipchain(t, "A", manager)
	c2, s2, a2 := makeSkipchain(t, "B", manager)
	conodes := Conodes{c1, c2}

	err := a1.InitChain(&empty.Empty{}, conodes)
	require.NoError(t, err)
	err = a2.InitChain(&empty.Empty{}, conodes)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	blocks := s2.Watch(ctx)

	for i := 0; i < n; i++ {
		err = a1.Store(&empty.Empty{}, conodes)
		require.NoError(t, err)

		event := <-blocks
		require.NotNil(t, event)
		require.IsType(t, SkipBlock{}, event)

		chain, err := s2.GetVerifiableBlock()
		require.NoError(t, err)

		packed, err := chain.Pack(s2.encoder)
		require.NoError(t, err)

		block, err := s2.GetBlockFactory().FromVerifiable(packed)
		require.NoError(t, err)
		require.NotNil(t, block)
		require.Equal(t, uint64(i+1), block.(SkipBlock).Index)
		require.Equal(t, event, block)
	}
}

func TestSkipchain_Listen(t *testing.T) {
	s := &Skipchain{
		mino:      fakeMino{},
		consensus: fakeConsensus{},
	}

	actor, err := s.Listen(nil)
	require.NoError(t, err)
	require.NotNil(t, actor)

	s.mino = fakeMino{err: xerrors.New("oops")}
	_, err = s.Listen(nil)
	require.EqualError(t, err, "couldn't create the rpc: oops")

	s.mino = fakeMino{}
	s.consensus = fakeConsensus{err: xerrors.New("oops")}
	_, err = s.Listen(nil)
	require.EqualError(t, err, "couldn't start the consensus: oops")
}

func TestSkipchain_GetBlock(t *testing.T) {
	digest := Digest{1, 2, 3}
	s := &Skipchain{
		db: &fakeDatabase{genesisID: digest},
	}

	block, err := s.GetBlock()
	require.NoError(t, err)
	require.Equal(t, digest, block.(SkipBlock).hash)

	s.db = &fakeDatabase{err: xerrors.New("oops")}
	_, err = s.GetBlock()
	require.EqualError(t, err, "couldn't read the latest block: oops")
}

func TestSkipchain_GetVerifiableBlock(t *testing.T) {
	digest := Digest{1, 2, 3}
	s := &Skipchain{
		db:        &fakeDatabase{genesisID: digest},
		consensus: fakeConsensus{},
	}

	block, err := s.GetVerifiableBlock()
	require.NoError(t, err)
	require.Equal(t, digest, block.(VerifiableBlock).hash)
	require.NotNil(t, block.(VerifiableBlock).Chain)

	s.db = &fakeDatabase{err: xerrors.New("oops")}
	_, err = s.GetVerifiableBlock()
	require.EqualError(t, err, "couldn't read the latest block: oops")

	s.db = &fakeDatabase{}
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
	actor := skipchainActor{
		hashFactory: sha256Factory{},
		rand:        crypto.CryptographicRandomGenerator{},
		Skipchain: &Skipchain{
			encoder: encoding.NewProtoEncoder(),
			mino:    fakeMino{},
			db:      &fakeDatabase{err: NewNoBlockError(0)},
		},
		rpc: fakeRPC{},
	}

	conodes := Conodes{randomConode()}
	conodes[0].addr = fakeAddress{id: []byte{0xaa}}

	err := actor.InitChain(&empty.Empty{}, conodes)
	require.NoError(t, err)

	err = actor.InitChain(&empty.Empty{}, fakePlayers{})
	require.EqualError(t, err, "players must implement cosi.CollectiveAuthority")

	actor.rpc = fakeRPC{err: xerrors.New("oops")}
	err = actor.InitChain(&empty.Empty{}, conodes)
	require.EqualError(t, xerrors.Unwrap(err), "couldn't propagate: oops")

	// No error so the genesis block exists already.
	actor.Skipchain.db = &fakeDatabase{}
	err = actor.InitChain(&empty.Empty{}, conodes)
	require.NoError(t, err)

	// Unexpected database error
	actor.Skipchain.db = &fakeDatabase{err: xerrors.New("oops")}
	err = actor.InitChain(&empty.Empty{}, conodes)
	require.EqualError(t, err, "couldn't read the genesis block: oops")
}

func TestActor_NewChain(t *testing.T) {
	actor := skipchainActor{
		hashFactory: sha256Factory{},
		rand:        crypto.CryptographicRandomGenerator{},
		Skipchain: &Skipchain{
			db: &fakeDatabase{},
		},
		rpc: fakeRPC{},
	}

	actor.rand = fakeRandGenerator{err: xerrors.New("oops")}
	err := actor.newChain(&empty.Empty{}, Conodes{})
	require.EqualError(t, err, "couldn't generate backlink: oops")

	actor.rand = fakeRandGenerator{noSize: true}
	err = actor.newChain(&empty.Empty{}, Conodes{})
	require.EqualError(t, err, "mismatch rand length 0 != 32")

	actor.rand = crypto.CryptographicRandomGenerator{}
	actor.hashFactory = badHashFactory{}
	err = actor.newChain(&empty.Empty{}, Conodes{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "couldn't create block: ")
}

func TestActor_Store(t *testing.T) {
	buffer := new(bytes.Buffer)
	cons := &fakeConsensusActor{}
	actor := skipchainActor{
		Skipchain: &Skipchain{
			encoder:    encoding.NewProtoEncoder(),
			logger:     zerolog.New(buffer),
			viewchange: fakeViewChange{},
			mino:       fakeMino{},
			db:         &fakeDatabase{},
		},
		consensus: cons,
	}

	conodes := Conodes{
		{addr: fakeAddress{id: []byte{0xbb}}},
		{addr: fakeAddress{id: []byte{0xaa}}},
		{addr: fakeAddress{id: []byte{0xcc}}},
	}

	err := actor.Store(&empty.Empty{}, conodes)
	require.NoError(t, err)
	// Make sure the conodes rotate if the view change allows it.
	require.NotNil(t, cons.prop)
	prop := cons.prop.(SkipBlock)
	require.Equal(t, prop.Conodes[0].GetAddress(), conodes[1].GetAddress())

	err = actor.Store(&empty.Empty{}, fakePlayers{})
	require.EqualError(t, err, "players must implement cosi.CollectiveAuthority")

	actor.Skipchain.db = &fakeDatabase{err: xerrors.New("oops")}
	err = actor.Store(&empty.Empty{}, conodes)
	require.EqualError(t, err, "couldn't read the latest block: oops")

	actor.Skipchain.db = &fakeDatabase{}
	actor.Skipchain.viewchange = fakeViewChange{err: xerrors.New("oops")}
	err = actor.Store(&empty.Empty{}, conodes)
	// A view change is ignored.
	require.NoError(t, err)
	require.Contains(t, buffer.String(), "skipchain@aa refusing view change: oops")

	actor.Skipchain.viewchange = fakeViewChange{}
	actor.consensus = &fakeConsensusActor{err: xerrors.New("oops")}
	err = actor.Store(&empty.Empty{}, conodes)
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

func makeSkipchain(t *testing.T, id string, manager *minoch.Manager) (Conode, *Skipchain, blockchain.Actor) {
	mino, err := minoch.NewMinoch(manager, id)
	require.NoError(t, err)

	signer := bls.NewSigner()

	conode := Conode{
		addr:      mino.GetAddress(),
		publicKey: signer.GetPublicKey(),
	}

	cosi := flatcosi.NewFlat(mino, signer)
	skipchain := NewSkipchain(mino, cosi)

	actor, err := skipchain.Listen(testValidator{})
	require.NoError(t, err)

	return conode, skipchain, actor
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

type fakePlayers struct {
	mino.Players
}

type fakeConsensusActor struct {
	consensus.Actor
	err  error
	prop consensus.Proposal
}

func (a *fakeConsensusActor) Propose(prop consensus.Proposal, pp mino.Players) error {
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

type fakeViewChange struct {
	err error
}

func (vc fakeViewChange) Wait(block blockchain.Block) (mino.Players, error) {
	// Simulate a rotating view change.
	players := block.GetPlayers().
		Take(mino.RangeFilter(0, block.GetPlayers().Len()), mino.RotateFilter(1))
	return players, vc.err
}

func (vc fakeViewChange) Verify(blockchain.Block) error {
	return vc.err
}
