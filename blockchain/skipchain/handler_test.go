package skipchain

import (
	"testing"
	"testing/quick"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

func TestHandler_Process(t *testing.T) {
	f := func(block SkipBlock) bool {
		h := newHandler(&Skipchain{
			db:        fakeDatabase{},
			cosi:      fakeCosi{},
			mino:      fakeMino{},
			consensus: fakeConsensus{},
		})

		packed, err := block.Pack()
		require.NoError(t, err)

		resp, err := h.Process(&PropagateGenesis{Genesis: packed.(*BlockProto)})
		require.NoError(t, err)
		require.Nil(t, resp)

		_, err = h.Process(&empty.Empty{})
		require.EqualError(t, err, "unknown message type '*empty.Empty'")

		_, err = h.Process(&PropagateGenesis{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "couldn't decode the block: ")

		h.Skipchain.db = fakeDatabase{err: xerrors.New("oops")}
		_, err = h.Process(&PropagateGenesis{Genesis: packed.(*BlockProto)})
		require.EqualError(t, err, "couldn't write the block: oops")

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}
