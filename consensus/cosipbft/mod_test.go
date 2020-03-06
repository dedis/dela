package cosipbft

import (
	"testing"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/cosi/blscosi"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/fabric/mino/minoch"
)

func TestConsensus_Basic(t *testing.T) {
	manager := minoch.NewManager()
	m1, err := minoch.NewMinoch(manager, "A")
	require.NoError(t, err)

	cosi := blscosi.NewBlsCoSi(m1, blscosi.NewSigner())

	cons := NewCoSiPBFT(m1, cosi)
	err = cons.Listen(fakeValidator{})
	require.NoError(t, err)

	err = cons.Propose(fakeProposal{}, fakeParticipant{
		addr:      m1.Address(),
		publicKey: cosi.GetPublicKey(),
	})
	require.NoError(t, err)
	require.Len(t, cons.links, 1)
}

type fakeProposal struct{}

func (p fakeProposal) Pack() (proto.Message, error) {
	return &empty.Empty{}, nil
}

func (p fakeProposal) GetHash() []byte {
	return nil
}

type fakeValidator struct{}

func (v fakeValidator) Validate(previous []byte, msg proto.Message) (consensus.Proposal, error) {
	return fakeProposal{}, nil
}

type fakeParticipant struct {
	addr      *mino.Address
	publicKey crypto.PublicKey
}

func (p fakeParticipant) Address() *mino.Address {
	return p.addr
}

func (p fakeParticipant) GetPublicKey() crypto.PublicKey {
	return p.publicKey
}
