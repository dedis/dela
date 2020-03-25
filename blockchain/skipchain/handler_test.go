package skipchain

import (
	"testing"
	"testing/quick"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

func TestHandler_Process(t *testing.T) {
	f := func(block SkipBlock) bool {
		h := newHandler(&Skipchain{
			db:        &fakeDatabase{},
			cosi:      fakeCosi{},
			mino:      fakeMino{},
			consensus: fakeConsensus{},
		})

		packed, err := block.Pack()
		require.NoError(t, err)

		req := mino.Request{
			Message: &PropagateGenesis{Genesis: packed.(*BlockProto)},
		}
		resp, err := h.Process(req)
		require.NoError(t, err)
		require.Nil(t, resp)

		req.Message = &empty.Empty{}
		_, err = h.Process(req)
		require.EqualError(t, err, "unknown message type '*empty.Empty'")

		req.Message = &PropagateGenesis{}
		_, err = h.Process(req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "couldn't decode the block: ")

		h.Skipchain.db = &fakeDatabase{err: xerrors.New("oops")}
		req.Message = &PropagateGenesis{Genesis: packed.(*BlockProto)}
		_, err = h.Process(req)
		require.EqualError(t, err, "couldn't write the block: oops")

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}
