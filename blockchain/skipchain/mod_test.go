package skipchain

import (
	"testing"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/cosi/blscosi"
	"go.dedis.ch/fabric/mino/minoch"
)

func TestSkipchain_Basic(t *testing.T) {
	n := 5
	manager := minoch.NewManager()

	c1, s1 := makeSkipchain(t, "A", manager)
	c2, s2 := makeSkipchain(t, "B", manager)

	roster, err := blockchain.NewRoster(s1.signer, c1, c2)
	require.NoError(t, err)

	err = s1.initChain(roster)
	require.NoError(t, err)

	for i := 0; i < n; i++ {
		err = s2.Store(roster, &empty.Empty{})
		require.NoError(t, err)

		chain, err := s1.GetVerifiableChain()
		require.NoError(t, err)

		packed, err := chain.Pack()
		require.NoError(t, err)

		block, err := s1.GetBlockFactory().FromVerifiable(packed, roster)
		require.NoError(t, err)
		require.NotNil(t, block)
		require.Equal(t, uint64(i+1), block.(SkipBlock).Index)
	}
}

type testValidator struct{}

func (v testValidator) Validate(payload proto.Message) error {
	return nil
}

func makeSkipchain(t *testing.T, id string, manager *minoch.Manager) (*blockchain.Conode, *Skipchain) {
	m1, err := minoch.NewMinoch(manager, id)
	require.NoError(t, err)

	i1 := blscosi.NewSigner()
	pubkey, err := i1.PublicKey().Pack()
	require.NoError(t, err)

	pubkeyany, err := ptypes.MarshalAny(pubkey)
	require.NoError(t, err)

	conode := &blockchain.Conode{
		Address:   m1.Address(),
		PublicKey: pubkeyany,
	}

	s1, err := NewSkipchain(m1, i1, testValidator{})
	require.NoError(t, err)

	return conode, s1
}
