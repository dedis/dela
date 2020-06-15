package skipchain

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde/tmp"
	"golang.org/x/xerrors"
)

func TestOperations_CatchUp(t *testing.T) {
	block := SkipBlock{
		Index:   5,
		Payload: fake.Message{},
	}

	rcv := fake.Receiver{Msg: tmp.ProtoOf(BlockResponse{block: block})}

	call := &fake.Call{}
	ops := &operations{
		addr:            fake.NewAddress(0),
		blockFactory:    NewBlockFactory(fake.MessageFactory{}),
		responseFactory: responseFactory{blockFactory: NewBlockFactory(fake.MessageFactory{})},
		reactor:         &fakeReactor{},
		db:              &fakeDatabase{blocks: []SkipBlock{{}}},
		watcher:         &fakeWatcher{call: call},
		rpc:             fake.NewStreamRPC(rcv, fake.Sender{}),
	}

	// No catch up for the genesis block.
	err := ops.catchUp(0, fake.NewAddress(0))
	require.NoError(t, err)
	require.Equal(t, 0, call.Len())

	// Normal catch up with more than one block missing.
	err = ops.catchUp(5, fake.NewAddress(0))
	require.NoError(t, err)
	require.Equal(t, 0, call.Len())

	// Catch up with only one block missing so it waits until timeout but the
	// block is committed right after the timeout.
	go func() {
		ops.catchUpLock.Lock()
		ops.db = &fakeDatabase{blocks: []SkipBlock{{}, {}, {}}}
		ops.catchUpLock.Unlock()
	}()
	err = ops.catchUp(2, fake.NewAddress(0))
	require.NoError(t, err)
	require.Equal(t, 2, call.Len())

	// Catch up with only one block missing but it arrives during the catch up.
	ops.db = &fakeDatabase{blocks: []SkipBlock{{}}}
	ops.watcher = &fakeWatcher{call: call, block: SkipBlock{Index: 1}}
	err = ops.catchUp(2, fake.NewAddress(0))
	require.NoError(t, err)
	require.Equal(t, 4, call.Len())

	ops.db = &fakeDatabase{blocks: []SkipBlock{{}}, err: xerrors.New("oops")}
	err = ops.catchUp(2, nil)
	require.EqualError(t, err, "couldn't read last block: oops")

	ops.db = &fakeDatabase{blocks: []SkipBlock{{}}}
	ops.rpc = fake.NewBadStreamRPC()
	err = ops.catchUp(5, nil)
	require.EqualError(t, err, "couldn't open stream: fake error")

	ops.rpc = fake.NewStreamRPC(fake.NewBadReceiver(), fake.Sender{})
	err = ops.catchUp(5, nil)
	require.EqualError(t, err, "couldn't receive message: fake error")

	ops.rpc = fake.NewStreamRPC(rcv, fake.Sender{})
	ops.reactor = &fakeReactor{errCommit: xerrors.New("oops")}
	err = ops.catchUp(5, nil)
	require.EqualError(t, err,
		"couldn't store block: tx failed: couldn't commit block: oops")

	ops.responseFactory = fake.MessageFactory{}
	ops.rpc = fake.NewStreamRPC(fake.Receiver{Msg: tmp.ProtoOf(fake.Message{})}, fake.Sender{})
	err = ops.catchUp(5, nil)
	require.EqualError(t, err, "invalid response type 'fake.Message' != 'skipchain.BlockResponse'")
}
