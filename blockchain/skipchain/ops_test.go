package skipchain

import (
	"testing"

	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/encoding"
	"go.dedis.ch/dela/internal/testing/fake"
	"golang.org/x/xerrors"
)

func TestOperations_InsertBlock(t *testing.T) {
	ops := &operations{
		blockFactory: blockFactory{
			encoder:     encoding.NewProtoEncoder(),
			hashFactory: crypto.NewSha256Factory(),
		},
		processor: &fakePayloadProc{},
		db:        &fakeDatabase{},
		watcher:   &fakeWatcher{},
	}

	block := SkipBlock{Index: 4}

	err := ops.insertBlock(block)
	require.NoError(t, err)
	require.Equal(t, uint64(4), block.GetIndex())

	ops.processor = &fakePayloadProc{errValidate: xerrors.New("oops")}
	err = ops.insertBlock(block)
	require.EqualError(t, err, "couldn't validate block: oops")

	ops.processor = &fakePayloadProc{errCommit: xerrors.New("oops")}
	err = ops.insertBlock(block)
	require.EqualError(t, err, "tx failed: couldn't commit block: oops")
}

func TestOperations_CatchUp(t *testing.T) {
	block := SkipBlock{
		Index:   5,
		Payload: &empty.Empty{},
	}
	blockpb, err := block.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)

	rcv := fake.Receiver{Msg: &BlockResponse{
		Block: blockpb.(*BlockProto),
		Chain: &any.Any{},
	}}

	call := &fake.Call{}
	ops := &operations{
		addr: fake.NewAddress(0),
		blockFactory: blockFactory{
			encoder:     encoding.NewProtoEncoder(),
			hashFactory: crypto.NewSha256Factory(),
		},
		processor: &fakePayloadProc{},
		db:        &fakeDatabase{blocks: []SkipBlock{{}}},
		watcher:   &fakeWatcher{call: call},
		rpc:       fake.NewStreamRPC(rcv, fake.Sender{}),
	}

	// No catch up for the genesis block.
	err = ops.catchUp(0, fake.NewAddress(0))
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

	ops.rpc = fake.NewStreamRPC(fake.Receiver{}, fake.Sender{})
	err = ops.catchUp(5, nil)
	require.EqualError(t, err, "invalid response type '<nil>' != '*skipchain.BlockResponse'")

	ops.rpc = fake.NewStreamRPC(fake.Receiver{Msg: &BlockResponse{}}, fake.Sender{})
	err = ops.catchUp(5, nil)
	require.EqualError(t, err,
		"couldn't decode block: couldn't unmarshal payload: message is nil")

	ops.rpc = fake.NewStreamRPC(rcv, fake.Sender{})
	ops.processor = &fakePayloadProc{errValidate: xerrors.New("oops")}
	err = ops.catchUp(5, nil)
	require.EqualError(t, err, "couldn't store block: couldn't validate block: oops")
}
