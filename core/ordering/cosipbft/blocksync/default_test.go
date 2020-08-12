package blocksync

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/ordering/cosipbft/blockstore"
	"go.dedis.ch/dela/core/ordering/cosipbft/blocksync/types"
	"go.dedis.ch/dela/core/ordering/cosipbft/pbft"
	cosipbft "go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/txn/anon"
	"go.dedis.ch/dela/core/validation/simple"
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

	syncs, roster := makeNodes(t, n)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := syncs[0].Sync(ctx, roster)

	waitEvent(t, events, roster.Len(), roster.Len())

	storeBlocks(t, syncs[0].blocks, num)

	// Test only a subset of the roster to prepare for the next test.
	events = syncs[0].Sync(ctx, roster.Take(mino.RangeFilter(0, k)))

	waitEvent(t, events, k, k)

	for i := 0; i < k; i++ {
		require.Equal(t, uint64(num), syncs[i].blocks.Len(), strconv.Itoa(i))
	}

	// Test that two parrallel synchronizations for the same latest index don't
	// mix each other. Also test that already updated participants won't fail.
	ch1 := syncs[0].Sync(ctx, roster)
	ch2 := syncs[k-1].Sync(ctx, roster)

	waitEvent(t, ch1, n, n)
	waitEvent(t, ch2, n, n)

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
	rcvr := fake.NewReceiver()
	sender := fake.Sender{}

	sync := defaultSync{
		rpc:    fake.NewStreamRPC(rcvr, sender),
		blocks: blockstore.NewInMemory(),
	}

	ctx := context.Background()

	events := sync.Sync(ctx, mino.NewAddresses())
	waitEvent(t, events, 0, 0)

	buffer := new(bytes.Buffer)
	sync.logger = zerolog.New(buffer)
	sync.rpc = fake.NewBadRPC()
	events = sync.Sync(ctx, mino.NewAddresses())
	waitEvent(t, events, 0, 0)
	require.Contains(t, buffer.String(), "fake error")
	require.Contains(t, buffer.String(), "synchronization failed")
}

func TestDefaultSync_Routine(t *testing.T) {
	in := fake.NewReceiver(
		types.NewSyncRequest(0),
		types.NewSyncRequest(0),
		types.NewSyncAck(),
	)
	out := fake.Sender{}

	sync := defaultSync{
		rpc:    fake.NewStreamRPC(in, out),
		blocks: blockstore.NewInMemory(),
	}

	ctx := context.Background()
	events := make(chan Event, 1)

	err := sync.routine(ctx, mino.NewAddresses(fake.NewAddress(0)), events)
	require.NoError(t, err)
	<-events

	sync.rpc = fake.NewBadRPC()
	err = sync.routine(ctx, mino.NewAddresses(), nil)
	require.EqualError(t, err, "stream failed: fake error")

	sync.rpc = fake.NewStreamRPC(fake.NewReceiver(), fake.NewBadSender())
	err = sync.routine(ctx, mino.NewAddresses(), nil)
	require.EqualError(t, err, "announcement failed: fake error")

	sync.rpc = fake.NewStreamRPC(fake.NewBadReceiver(), fake.Sender{})
	err = sync.routine(ctx, mino.NewAddresses(fake.NewAddress(0)), nil)
	require.EqualError(t, err, "receiver failed: fake error")

	sync.blocks = badBlockStore{}
	sync.rpc = fake.NewStreamRPC(fake.NewReceiver(types.NewSyncRequest(0)), out)
	err = sync.routine(ctx, mino.NewAddresses(fake.NewAddress(0)), events)
	require.NoError(t, err)

	evt := <-events
	require.Len(t, evt.Errors, 1)
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
		latest: &latest,
		blocks: blockstore.NewInMemory(),
	}
	handler.pbftsm = testSM{blocks: handler.blocks}

	err := handler.Stream(fake.Sender{}, fake.NewReceiver(types.NewSyncMessage(0)))
	require.NoError(t, err)

	msgs := []serde.Message{types.NewSyncMessage(blocks.Len())}
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

	err = handler.Stream(fake.NewBadSender(), fake.NewReceiver(types.NewSyncMessage(6)))
	require.EqualError(t, err, "sending request failed: fake error")

	rcvr := fake.NewBadReceiver()
	rcvr.Msg = []serde.Message{types.NewSyncMessage(6)}
	err = handler.Stream(fake.Sender{}, rcvr)
	require.EqualError(t, err, "receiver failed: fake error")

	msgs = []serde.Message{types.NewSyncMessage(6), msgs[1]}
	err = handler.Stream(fake.Sender{}, fake.NewReceiver(msgs...))
	require.Error(t, err)
	require.Regexp(t, "pbft catch up failed: mismatch link '[0]{8}' != '[0-9a-f]{8}'", err.Error())

	err = handler.Stream(fake.NewBadSender(), fake.NewReceiver(types.NewSyncMessage(0)))
	require.EqualError(t, err, "sending ack failed: fake error")
}

// Utility functions -----------------------------------------------------------

func makeNodes(t *testing.T, n int) ([]defaultSync, mino.Players) {
	manager := minoch.NewManager()

	syncs := make([]defaultSync, n)
	addrs := make([]mino.Address, n)

	for i := 0; i < n; i++ {
		m, err := minoch.NewMinoch(manager, fmt.Sprintf("node%d", i))
		require.NoError(t, err)

		addrs[i] = m.GetAddress()

		blocks := blockstore.NewInMemory()
		blockFac := cosipbft.NewBlockFactory(simple.NewDataFactory(anon.NewTransactionFactory()))

		param := SyncParam{
			Mino:        m,
			Blocks:      blocks,
			LinkFactory: cosipbft.NewBlockLinkFactory(blockFac, fake.SignatureFactory{}),
			PBFT:        testSM{blocks: blocks},
		}

		sync, err := NewSynchronizer(param)
		require.NoError(t, err)

		syncs[i] = sync.(defaultSync)
	}

	return syncs, mino.NewAddresses(addrs...)
}

func storeBlocks(t *testing.T, blocks blockstore.BlockStore, n int) {
	prev := cosipbft.Digest{}

	for i := 0; i < n; i++ {
		block, err := cosipbft.NewBlock(simple.NewData(nil), cosipbft.WithIndex(uint64(i)))
		require.NoError(t, err)

		err = blocks.Store(cosipbft.NewBlockLink(prev, block, fake.Signature{}, fake.Signature{}))
		require.NoError(t, err)

		prev = block.GetHash()
	}
}

func waitEvent(t *testing.T, events <-chan Event, soft, hard int) {
	for {
		select {
		case <-time.After(2 * time.Second):
			t.Fatal("timeout")
		case evt := <-events:
			if evt.Soft >= soft && evt.Hard >= hard {
				return
			}
		}
	}
}

type testSM struct {
	pbft.StateMachine

	blocks blockstore.BlockStore
}

func (sm testSM) CatchUp(link cosipbft.BlockLink) error {
	err := sm.blocks.Store(link)
	if err != nil {
		return err
	}

	return nil
}

type badBlockStore struct {
	blockstore.BlockStore
}

func (s badBlockStore) Len() uint64 {
	return 5
}

func (s badBlockStore) GetByIndex(index uint64) (cosipbft.BlockLink, error) {
	return nil, xerrors.New("oops")
}
