package skipchain

import (
	"testing"

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

	base := SkipBlock{Index: 4, Payload: &empty.Empty{}}
	basepb, err := base.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)

	block, err := ops.insertBlock(basepb)
	require.NoError(t, err)
	require.Equal(t, uint64(4), block.GetIndex())

	_, err = ops.insertBlock(nil)
	require.EqualError(t, err, "couldn't decode block: invalid message type '<nil>'")

	ops.processor = &fakePayloadProc{errValidate: xerrors.New("oops")}
	_, err = ops.insertBlock(basepb)
	require.EqualError(t, err, "couldn't validate block: oops")

	ops.processor = &fakePayloadProc{errCommit: xerrors.New("oops")}
	_, err = ops.insertBlock(basepb)
	require.EqualError(t, err, "tx failed: couldn't commit block: oops")
}

func TestOperations_CatchUp(t *testing.T) {
	block := SkipBlock{
		Index:   2,
		Payload: &empty.Empty{},
	}
	blockpb, err := block.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)

	rcv := fake.Receiver{Msg: &BlockResponse{Block: blockpb.(*BlockProto)}}

	ops := &operations{
		blockFactory: blockFactory{
			encoder:     encoding.NewProtoEncoder(),
			hashFactory: crypto.NewSha256Factory(),
		},
		processor: &fakePayloadProc{},
		db:        &fakeDatabase{missing: true},
		watcher:   &fakeWatcher{},
		rpc:       fake.NewStreamRPC(rcv, fake.Sender{}),
	}

	hash, err := block.computeHash(crypto.NewSha256Factory(), encoding.NewProtoEncoder())
	require.NoError(t, err)

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
		"couldn't store block: couldn't decode block: couldn't unmarshal payload: message is nil")
}
