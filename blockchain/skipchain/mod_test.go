package skipchain

import (
	"testing"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/m/blockchain"
	"go.dedis.ch/m/cosi/blscosi"
	"go.dedis.ch/m/mino/minoch"
)

func TestSkipchain_Basic(t *testing.T) {
	manager := minoch.NewManager()

	c1, s1 := makeSkipchain(t, manager)

	roster := blockchain.Roster{c1}

	err := s1.initChain(roster)
	require.NoError(t, err)

	err = s1.Store(roster, &empty.Empty{})
	require.NoError(t, err)
}

type testValidator struct{}

func (v testValidator) Validate(SkipBlock) error {
	return nil
}

func makeSkipchain(t *testing.T, manager *minoch.Manager) (*blockchain.Conode, *Skipchain) {
	m1, err := minoch.NewMinoch(manager, "A")
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
