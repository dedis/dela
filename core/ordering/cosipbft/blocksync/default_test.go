package blocksync

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/core/ordering/cosipbft/blockstore"
	"go.dedis.ch/dela/core/ordering/cosipbft/blocksync/types"
	"go.dedis.ch/dela/core/ordering/cosipbft/pbft"
	otypes "go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/txn/signed"
	"go.dedis.ch/dela/core/validation/simple"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minoch"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func TestDefaultSync_Basic(t *testing.T) {
	n := 20
	k := 8
	num := 10

	syncs, genesis, roster := makeNodes(t, n)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := syncs[0].Sync(ctx, roster, Config{
		MinSoft: roster.Len(),
		MinHard: roster.Len(),
	})
	require.NoError(t, err)

	storeBlocks(t, syncs[0].blocks, num, genesis.GetHash().Bytes()...)

	// Test only a subset of the roster to prepare for the next test.
	err = syncs[0].Sync(ctx, roster.Take(mino.RangeFilter(0, k)), Config{
		MinSoft: k,
		MinHard: k,
	})
	require.NoError(t, err)

	for i := 0; i < k; i++ {
		require.Equal(t, uint64(num), syncs[i].blocks.Len(), strconv.Itoa(i))
	}

	// Test that two parrallel synchronizations for the same latest index don't
	// mix each other. Also test that already updated participants won't fail.
	wg := sync.WaitGroup{}
	wg.Add(2)

	cfg := Config{
		MinSoft: n,
		MinHard: n,
	}

	go func() {
		defer wg.Done()
		require.NoError(t, syncs[0].Sync(ctx, roster, cfg))
	}()
	go func() {
		defer wg.Done()
		require.NoError(t, syncs[k-1].Sync(ctx, roster, cfg))
	}()

	wg.Wait()

	for i := 0; i < n; i++ {
		require.Equal(t, uint64(num), syncs[i].blocks.Len(), strconv.Itoa(i))
	}
}

func TestDefaultSync_New(t *testing.T) {
	param := SyncParam{
		Mino:   fake.NewBadMino(),
		Blocks: blockstore.NewInMemory(),
	}

	_, err := NewSynchronizer(param)
	require.EqualError(t, err, "rpc creation failed: fake error")
}

func TestDefaultSync_GetLatest(t *testing.T) {
	latest := uint64(5)

	sync := defaultSync{
		latest: &latest,
	}

	require.Equal(t, uint64(5), sync.GetLatest())
}

func TestDefaultSync_Sync(t *testing.T) {
	rcvr := fake.NewReceiver(types.NewSyncRequest(0), types.NewSyncRequest(0), types.NewSyncAck())
	sender := fake.Sender{}

	sync := defaultSync{
		rpc:    fake.NewStreamRPC(rcvr, sender),
		blocks: blockstore.NewInMemory(),
	}

	storeBlocks(t, sync.blocks, 1)

	ctx := context.Background()

	err := sync.Sync(ctx, mino.NewAddresses(), Config{MinSoft: 1, MinHard: 1})
	require.NoError(t, err)

	sync.blocks = badBlockStore{errChain: xerrors.New("oops")}
	err = sync.Sync(ctx, mino.NewAddresses(), Config{})
	require.EqualError(t, err, "failed to read chain: oops")

	sync.blocks = blockstore.NewInMemory()
	storeBlocks(t, sync.blocks, 1)
	sync.rpc = fake.NewBadRPC()
	err = sync.Sync(ctx, mino.NewAddresses(), Config{})
	require.EqualError(t, err, "stream failed: fake error")

	sync.rpc = fake.NewStreamRPC(fake.NewReceiver(), fake.NewBadSender())
	err = sync.Sync(ctx, mino.NewAddresses(), Config{})
	require.EqualError(t, err, "announcement failed: fake error")

	sync.rpc = fake.NewStreamRPC(fake.NewBadReceiver(), fake.Sender{})
	err = sync.Sync(ctx, mino.NewAddresses(fake.NewAddress(0)), Config{MinSoft: 1})
	require.EqualError(t, err, "receiver failed: fake error")

	sync.rpc = fake.NewStreamRPC(fake.NewReceiver(types.NewSyncRequest(0)), sender)
	sync.blocks = badBlockStore{}
	err = sync.Sync(ctx, mino.NewAddresses(), Config{MinSoft: 1})
	require.EqualError(t, err, "synchronizing node fake.Address[0]: couldn't get block: oops")
}

func TestDefaultSync_SyncNode(t *testing.T) {
	sync := defaultSync{
		blocks: blockstore.NewInMemory(),
	}

	storeBlocks(t, sync.blocks, 5)

	err := sync.syncNode(0, fake.NewBadSender(), fake.NewAddress(0))
	require.EqualError(t, err, "failed to send block: fake error")
}

func TestHandler_Stream(t *testing.T) {
	latest := uint64(0)
	blocks := blockstore.NewInMemory()
	storeBlocks(t, blocks, 3)

	handler := &handler{
		latest:      &latest,
		genesis:     blockstore.NewGenesisStore(),
		blocks:      blockstore.NewInMemory(),
		verifierFac: fake.VerifierFactory{},
	}
	handler.genesis.Set(otypes.Genesis{})
	handler.pbftsm = testSM{blocks: handler.blocks}

	err := handler.Stream(fake.Sender{}, fake.NewReceiver(types.NewSyncMessage(makeChain(t, 0))))
	require.NoError(t, err)

	msgs := []serde.Message{types.NewSyncMessage(makeChain(t, blocks.Len()))}
	for i := uint64(0); i < blocks.Len(); i++ {
		link, err := blocks.GetByIndex(i)
		require.NoError(t, err)

		msgs = append(msgs, types.NewSyncReply(link))
	}

	err = handler.Stream(fake.Sender{}, fake.NewReceiver(msgs...))
	require.NoError(t, err)
	require.Equal(t, blocks.Len(), handler.blocks.Len())

	err = handler.Stream(fake.Sender{}, fake.NewBadReceiver())
	require.EqualError(t, err, "no announcement: receiver failed: fake error")

	handler.genesis = blockstore.NewGenesisStore()
	err = handler.Stream(fake.Sender{}, fake.NewReceiver(msgs...))
	require.EqualError(t, err, "reading genesis: missing genesis block")

	handler.genesis.Set(otypes.Genesis{})
	err = handler.Stream(fake.Sender{}, fake.NewReceiver(types.NewSyncMessage(fakeChain{err: xerrors.New("oops")})))
	require.EqualError(t, err, "failed to verify chain: oops")

	err = handler.Stream(fake.NewBadSender(), fake.NewReceiver(types.NewSyncMessage(makeChain(t, 6))))
	require.EqualError(t, err, "sending request failed: fake error")

	rcvr := fake.NewBadReceiver()
	rcvr.Msg = []serde.Message{types.NewSyncMessage(makeChain(t, 6))}
	err = handler.Stream(fake.Sender{}, rcvr)
	require.EqualError(t, err, "receiver failed: fake error")

	msgs = []serde.Message{types.NewSyncMessage(makeChain(t, 6)), msgs[1]}
	err = handler.Stream(fake.Sender{}, fake.NewReceiver(msgs...))
	require.Error(t, err)
	require.Regexp(t, "pbft catch up failed: mismatch link '[0]{8}' != '[0-9a-f]{8}'", err.Error())

	err = handler.Stream(fake.NewBadSender(), fake.NewReceiver(types.NewSyncMessage(makeChain(t, 0))))
	require.EqualError(t, err, "sending ack failed: fake error")
}

// -----------------------------------------------------------------------------
// Utility functions

func makeChain(t *testing.T, index uint64) otypes.Chain {
	block, err := otypes.NewBlock(simple.NewData(nil), otypes.WithIndex(index))
	require.NoError(t, err)

	return fakeChain{block: block}
}

func makeNodes(t *testing.T, n int) ([]defaultSync, otypes.Genesis, mino.Players) {
	manager := minoch.NewManager()

	syncs := make([]defaultSync, n)
	addrs := make([]mino.Address, n)

	ro := authority.FromAuthority(fake.NewAuthority(3, fake.NewSigner))

	genesis, err := otypes.NewGenesis(ro)
	require.NoError(t, err)

	for i := 0; i < n; i++ {
		m, err := minoch.NewMinoch(manager, fmt.Sprintf("node%d", i))
		require.NoError(t, err)

		addrs[i] = m.GetAddress()

		genstore := blockstore.NewGenesisStore()
		genstore.Set(genesis)

		blocks := blockstore.NewInMemory()
		blockFac := otypes.NewBlockFactory(simple.NewDataFactory(signed.NewTransactionFactory()))
		csFac := authority.NewChangeSetFactory(m.GetAddressFactory(), fake.PublicKeyFactory{})
		linkFac := otypes.NewLinkFactory(blockFac, fake.SignatureFactory{}, csFac)

		param := SyncParam{
			Mino:            m,
			Blocks:          blocks,
			Genesis:         genstore,
			LinkFactory:     linkFac,
			ChainFactory:    otypes.NewChainFactory(linkFac),
			PBFT:            testSM{blocks: blocks},
			VerifierFactory: fake.VerifierFactory{},
		}

		sync, err := NewSynchronizer(param)
		require.NoError(t, err)

		syncs[i] = sync.(defaultSync)
	}

	return syncs, genesis, mino.NewAddresses(addrs...)
}

func storeBlocks(t *testing.T, blocks blockstore.BlockStore, n int, from ...byte) {
	prev := otypes.Digest{}
	copy(prev[:], from)

	for i := 0; i < n; i++ {
		block, err := otypes.NewBlock(simple.NewData(nil), otypes.WithIndex(uint64(i+1)))
		require.NoError(t, err)

		link, err := otypes.NewBlockLink(prev, block,
			otypes.WithSignatures(fake.Signature{}, fake.Signature{}))
		require.NoError(t, err)

		err = blocks.Store(link)
		require.NoError(t, err)

		prev = block.GetHash()
	}
}

type testSM struct {
	pbft.StateMachine

	blocks blockstore.BlockStore
}

func (sm testSM) CatchUp(link otypes.BlockLink) error {
	err := sm.blocks.Store(link)
	if err != nil {
		return err
	}

	return nil
}

type badBlockStore struct {
	blockstore.BlockStore

	errChain error
}

func (s badBlockStore) Len() uint64 {
	return 5
}

func (s badBlockStore) GetChain() (otypes.Chain, error) {
	return nil, s.errChain
}

func (s badBlockStore) GetByIndex(index uint64) (otypes.BlockLink, error) {
	return nil, xerrors.New("oops")
}

type fakeChain struct {
	otypes.Chain

	block otypes.Block
	err   error
}

func (c fakeChain) GetBlock() otypes.Block {
	return c.block
}

func (c fakeChain) Verify(otypes.Genesis, crypto.VerifierFactory) error {
	return c.err
}
