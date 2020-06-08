package cosipbft

import (
	"testing"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/encoding"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestPrepare_Pack(t *testing.T) {
	req := Prepare{
		message:   &empty.Empty{},
		signature: fake.Signature{},
		chain:     forwardLinkChain{},
	}

	reqpb, err := req.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.NotNil(t, reqpb)

	_, err = req.Pack(fake.BadMarshalAnyEncoder{})
	require.EqualError(t, err, "couldn't pack proposal: fake error")

	_, err = req.Pack(fake.BadPackAnyEncoder{})
	require.EqualError(t, err, "couldn't pack signature: fake error")

	_, err = req.Pack(fake.BadPackAnyEncoder{Counter: fake.NewCounter(1)})
	require.EqualError(t, err, "couldn't pack chain: fake error")
}

func TestCommit_Pack(t *testing.T) {
	req := Commit{to: []byte{0xab}, prepare: fake.Signature{}}

	reqpb, err := req.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.NotNil(t, reqpb)

	_, err = req.Pack(fake.BadPackAnyEncoder{})
	require.EqualError(t, err, "couldn't pack prepare signature: fake error")
}
