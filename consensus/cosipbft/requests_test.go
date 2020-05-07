package cosipbft

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/internal/testing/fake"
)

func TestPrepare_Pack(t *testing.T) {
	req := Prepare{proposal: fakeProposal{}}

	reqpb, err := req.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.NotNil(t, reqpb)

	_, err = req.Pack(fake.BadPackAnyEncoder{})
	require.EqualError(t, err, "couldn't pack proposal: fake error")
}

func TestCommit_Pack(t *testing.T) {
	req := Commit{to: []byte{0xab}, prepare: fake.Signature{}}

	reqpb, err := req.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.NotNil(t, reqpb)

	_, err = req.Pack(fake.BadPackAnyEncoder{})
	require.EqualError(t, err, "couldn't pack prepare signature: fake error")
}
