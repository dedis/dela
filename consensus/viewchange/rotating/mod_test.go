package rotating

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/internal/testing/fake"
)

func TestViewChange_Wait(t *testing.T) {
	authority := fake.NewAuthority(4, fake.NewSigner)

	vc := NewViewChange(fake.NewAddress(0))

	index, isLeader := vc.Wait(fakeProposal{}, authority)
	require.Equal(t, uint32(0), index)
	require.True(t, isLeader)

	index, isLeader = vc.Wait(fakeProposal{index: 1}, authority)
	require.Equal(t, uint32(1), index)
	require.False(t, isLeader)

	index, isLeader = vc.Wait(fakeProposal{index: 4}, authority)
	require.Equal(t, uint32(0), index)
	require.True(t, isLeader)
}

func TestViewChange_Verify(t *testing.T) {
	authority := fake.NewAuthority(3, fake.NewSigner)

	vc := NewViewChange(fake.NewAddress(0))

	leader := vc.Verify(fakeProposal{}, authority)
	require.Equal(t, uint32(0), leader)

	leader = vc.Verify(fakeProposal{index: 2}, authority)
	require.Equal(t, uint32(2), leader)

	leader = vc.Verify(fakeProposal{index: 3}, authority)
	require.Equal(t, uint32(0), leader)
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeProposal struct {
	consensus.Proposal
	index uint64
}

func (p fakeProposal) GetIndex() uint64 {
	return p.index
}
