package skipchain

import (
	"testing"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/cosi/blscosi"
	"go.dedis.ch/fabric/mino/minoch"
)

func TestSkipchain_Basic(t *testing.T) {
	n := 5
	manager := minoch.NewManager()

	c1, s1 := makeSkipchain(t, "A", manager)
	c2, s2 := makeSkipchain(t, "B", manager)
	conodes := Conodes{c1, c2}

	err := s1.initChain(conodes)
	require.NoError(t, err)

	for i := 0; i < n; i++ {
		err = s2.Store(&empty.Empty{})
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

func makeSkipchain(t *testing.T, id string, manager *minoch.Manager) (Conode, *Skipchain) {
	mino, err := minoch.NewMinoch(manager, id)
	require.NoError(t, err)

	signer := blscosi.NewSigner()

	conode := Conode{
		addr:      mino.Address(),
		publicKey: signer.PublicKey(),
	}

	cosi := blscosi.NewBlsCoSi(mino, signer)
	skipchain := NewSkipchain(mino, cosi, testValidator{})
	skipchain.Listen()

	return conode, skipchain
}
