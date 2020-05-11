package skipchain

import (
	fmt "fmt"
	"testing"

	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/internal/testing/fake"
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
		Index:   0,
		Payload: &empty.Empty{},
	}
	blockpb, err := block.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)

	rcv := fake.Receiver{Msg: &BlockResponse{
		Block: blockpb.(*BlockProto),
		Chain: &any.Any{},
	}}

	hash, err := block.computeHash(crypto.NewSha256Factory(), encoding.NewProtoEncoder())
	require.NoError(t, err)

	ops := &operations{
		blockFactory: blockFactory{
			encoder:     encoding.NewProtoEncoder(),
			hashFactory: crypto.NewSha256Factory(),
		},
		processor: &fakePayloadProc{},
		db:        &fakeDatabase{missing: true},
		watcher:   &fakeWatcher{},
		rpc:       fake.NewStreamRPC(rcv, fake.Sender{}),
		consensus: fakeConsensus{hash: hash},
	}

	err = ops.catchUp(SkipBlock{BackLink: hash}, fake.NewAddress(0))
	require.NoError(t, err)

	ops.rpc = fake.NewStreamRPC(fake.NewBadReceiver(), fake.Sender{})
	err = ops.catchUp(SkipBlock{}, nil)
	require.EqualError(t, err, "couldn't receive message: fake error")

	ops.rpc = fake.NewStreamRPC(fake.Receiver{}, fake.Sender{})
	err = ops.catchUp(SkipBlock{}, nil)
	require.EqualError(t, err, "invalid response type '<nil>' != '*skipchain.BlockResponse'")

	ops.rpc = fake.NewStreamRPC(fake.Receiver{Msg: &BlockResponse{}}, fake.Sender{})
	err = ops.catchUp(SkipBlock{}, nil)
	require.EqualError(t, err,
		"couldn't decode block: couldn't unmarshal payload: message is nil")

	ops.rpc = fake.NewStreamRPC(rcv, fake.Sender{})
	ops.processor = &fakePayloadProc{errValidate: xerrors.New("oops")}
	err = ops.catchUp(SkipBlock{}, nil)
	require.EqualError(t, err, "couldn't store block: couldn't validate block: oops")

	ops.rpc = fake.NewStreamRPC(rcv, fake.Sender{})
	ops.consensus = fakeConsensus{errFactory: xerrors.New("oops")}
	err = ops.catchUp(SkipBlock{}, nil)
	require.EqualError(t, err, "couldn't get chain factory: oops")

	ops.consensus = fakeConsensus{err: xerrors.New("oops")}
	err = ops.catchUp(SkipBlock{}, nil)
	require.EqualError(t, err, "couldn't decode chain: oops")

	ops.consensus = fakeConsensus{hash: Digest{0x01}}
	err = ops.catchUp(SkipBlock{}, nil)
	require.EqualError(t, err, fmt.Sprintf("mismatch chain: hash '%x' != '%x'",
		Digest{0x01}.Bytes(), hash.Bytes()))

	ops.consensus = fakeConsensus{hash: hash, errStore: xerrors.New("oops")}
	err = ops.catchUp(SkipBlock{}, nil)
	require.EqualError(t, err, "couldn't store chain: oops")

	blockpb.(*BlockProto).Index = 1
	rcv2 := fake.Receiver{Msg: &BlockResponse{Block: blockpb.(*BlockProto)}}
	ops.rpc = fake.NewStreamRPC(rcv2, fake.Sender{})
	err = ops.catchUp(SkipBlock{}, nil)
	require.EqualError(t, err, "missing chain to the block in the response")
}
