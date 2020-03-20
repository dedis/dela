package skipchain

import (
	"context"
	"testing"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/cosi/flatcosi"
	"go.dedis.ch/fabric/crypto/bls"
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

	c1, s1, a1 := makeSkipchain(t, "A", manager)
	c2, _, a2 := makeSkipchain(t, "B", manager)
	conodes := Conodes{c1, c2}

	err := a1.InitChain(&empty.Empty{}, conodes)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	blocks := s1.Watch(ctx)

	for i := 0; i < n; i++ {
		err = a2.Store(&empty.Empty{}, conodes)
		require.NoError(t, err)

		event := <-blocks
		require.NotNil(t, event)
		require.IsType(t, SkipBlock{}, event)

		chain, err := s1.GetVerifiableBlock()
		require.NoError(t, err)

		packed, err := chain.Pack()
		require.NoError(t, err)

		block, err := s1.GetBlockFactory().FromVerifiable(packed)
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
		Skipchain: &Skipchain{
			db: &fakeDatabase{},
		},
		rpc: fakeRPC{},
	}

	err := actor.InitChain(&empty.Empty{}, Conodes{})
	require.NoError(t, err)

	err = actor.InitChain(&empty.Empty{}, fakePlayers{})
	require.EqualError(t, err, "players must implement cosi.CollectiveAuthority")

	actor.hashFactory = badHashFactory{}
	err = actor.InitChain(&empty.Empty{}, Conodes{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "couldn't create block: ")

	actor.hashFactory = sha256Factory{}
	actor.Skipchain.db = &fakeDatabase{err: xerrors.New("oops")}
	err = actor.InitChain(&empty.Empty{}, Conodes{})
	require.EqualError(t, err, "couldn't write genesis block: oops")

	actor.Skipchain.db = &fakeDatabase{}
	actor.rpc = fakeRPC{err: xerrors.New("oops")}
	err = actor.InitChain(&empty.Empty{}, Conodes{})
	require.EqualError(t, err, "couldn't propagate: oops")
}

func TestActor_Store(t *testing.T) {
	actor := skipchainActor{
		Skipchain: &Skipchain{
			db: &fakeDatabase{},
		},
		consensus: fakeConsensusActor{},
	}

	err := actor.Store(&empty.Empty{}, Conodes{})
	require.NoError(t, err)

	actor.Skipchain.db = &fakeDatabase{err: xerrors.New("oops")}
	err = actor.Store(&empty.Empty{}, Conodes{})
	require.EqualError(t, err, "couldn't read the latest block: oops")

	actor.Skipchain.db = &fakeDatabase{}
	actor.consensus = fakeConsensusActor{err: xerrors.New("oops")}
	err = actor.Store(&empty.Empty{}, Conodes{})
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

func (v testValidator) Validate(payload proto.Message) error {
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

func (rpc fakeRPC) Call(proto.Message, mino.Players) (<-chan proto.Message, <-chan error) {
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
	err error
}

func (a fakeConsensusActor) Propose(consensus.Proposal, mino.Players) error {
	return a.err
}
