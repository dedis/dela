package skipchain

import (
	"testing"
	"testing/quick"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

func TestHandler_Process(t *testing.T) {
	f := func(block SkipBlock) bool {
		proc := &fakePayloadProc{}
		h := newHandler(&Skipchain{
			encoder:   encoding.NewProtoEncoder(),
			db:        &fakeDatabase{},
			cosi:      fakeCosi{},
			mino:      fakeMino{},
			consensus: fakeConsensus{},
		}, proc)

		block.Payload = &wrappers.BoolValue{Value: true}

		packed, err := block.Pack(encoding.NewProtoEncoder())
		require.NoError(t, err)

		req := mino.Request{
			Message: &PropagateGenesis{Genesis: packed.(*BlockProto)},
		}
		resp, err := h.Process(req)
		require.NoError(t, err)
		require.Nil(t, resp)
		require.Len(t, proc.calls, 2)
		require.Equal(t, uint64(0), proc.calls[0][0])
		require.True(t, proto.Equal(block.Payload, proc.calls[0][1].(proto.Message)))
		require.True(t, proto.Equal(block.Payload, proc.calls[1][0].(proto.Message)))

		req.Message = &empty.Empty{}
		_, err = h.Process(req)
		require.EqualError(t, err, "unknown message type '*empty.Empty'")

		req.Message = &PropagateGenesis{}
		_, err = h.Process(req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "couldn't decode the block: ")

		req.Message = &PropagateGenesis{Genesis: packed.(*BlockProto)}
		h.proc = &fakePayloadProc{errValidate: xerrors.New("oops")}
		_, err = h.Process(req)
		require.EqualError(t, err, "couldn't validate genesis payload: oops")

		h.proc = &fakePayloadProc{errCommit: xerrors.New("oops")}
		_, err = h.Process(req)
		require.EqualError(t, err, "tx aborted: couldn't commit genesis payload: oops")

		h.proc = &fakePayloadProc{}
		h.Skipchain.db = &fakeDatabase{err: xerrors.New("oops")}
		_, err = h.Process(req)
		require.EqualError(t, err, "tx aborted: couldn't write the block: oops")

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}
