package skipchain

import (
	"testing"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/cosi/flatcosi"
	"go.dedis.ch/fabric/crypto/bls"
	internal "go.dedis.ch/fabric/internal/testing"
	"go.dedis.ch/fabric/mino/minoch"
)

func TestMessages(t *testing.T) {
	messages := []proto.Message{
		&ConodeProto{},
		&BlockProto{},
		&VerifiableBlockProto{},
		&PropagateGenesis{},
	}

	for _, m := range messages {
		internal.CoverProtoMessage(t, m)
	}
}

func TestSkipchain_Basic(t *testing.T) {
	n := 5
	manager := minoch.NewManager()

	c1, s1, a1 := makeSkipchain(t, "A", manager)
	c2, _, a2 := makeSkipchain(t, "B", manager)
	conodes := Conodes{c1, c2}

	err := a1.InitChain(&empty.Empty{}, conodes)
	require.NoError(t, err)

	for i := 0; i < n; i++ {
		err = a2.Store(&empty.Empty{}, conodes)
		require.NoError(t, err)

		chain, err := s1.GetVerifiableBlock()
		require.NoError(t, err)

		packed, err := chain.Pack()
		require.NoError(t, err)

		block, err := s1.GetBlockFactory().FromVerifiable(packed)
		require.NoError(t, err)
		require.NotNil(t, block)
		require.Equal(t, uint64(i+1), block.(SkipBlock).Index)
	}
}

type testValidator struct{}

func (v testValidator) Validate(payload proto.Message) error {
	return nil
}

func (v testValidator) Commit(payload proto.Message) error {
	return nil
}

func makeSkipchain(t *testing.T, id string, manager *minoch.Manager) (Conode, *Skipchain, blockchain.Actor) {
	mino, err := minoch.NewMinoch(manager, id)
	require.NoError(t, err)

	signer := bls.NewSigner()

	conode := Conode{
		addr:      mino.GetAddress(),
		publicKey: signer.GetPublicKey(),
	}

	cosi := flatcosi.NewFlat(mino, signer)
	skipchain := NewSkipchain(mino, cosi)

	actor, err := skipchain.Listen(testValidator{})
	require.NoError(t, err)

	return conode, skipchain, actor
}
