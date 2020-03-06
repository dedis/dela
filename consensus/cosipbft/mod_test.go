package cosipbft

import (
	"testing"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/cosi/blscosi"
	"go.dedis.ch/fabric/mino/minoch"
)

func TestConsensus_Basic(t *testing.T) {
	manager := minoch.NewManager()
	m1, err := minoch.NewMinoch(manager, "A")
	require.NoError(t, err)

	cosi := blscosi.NewBlsCoSi(m1, blscosi.NewSigner())

	cons := NewCoSiPBFT(cosi)
	err = cons.Listen(nil)
	require.NoError(t, err)

	err = cons.Propose(fakeProposal{})
	require.NoError(t, err)
}

type fakeProposal struct{}

func (p fakeProposal) Pack() (proto.Message, error) {
	return &empty.Empty{}, nil
}

func (p fakeProposal) GetHash() []byte {
	return nil
}
