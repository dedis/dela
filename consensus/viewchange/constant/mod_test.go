package constant

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestConstant_GetChangeSetFactory(t *testing.T) {
	authority := roster.FromAuthority(fake.NewAuthority(3, fake.NewSigner))
	vc := NewViewChange(fake.NewAddress(0), authority, fakeChangeSetFactory{})
	require.NotNil(t, vc.GetChangeSetFactory())
}

func TestConstant_GetAuthority(t *testing.T) {
	authority := roster.FromAuthority(fake.NewAuthority(3, fake.NewSigner))
	vc := NewViewChange(fake.NewAddress(0), authority, fakeChangeSetFactory{})

	ret, err := vc.GetAuthority(0)
	require.NoError(t, err)
	require.Equal(t, authority, ret)

	ret, err = vc.GetAuthority(666)
	require.NoError(t, err)
	require.Equal(t, authority, ret)
}

func TestConstant_Wait(t *testing.T) {
	authority := fake.NewAuthority(3, fake.NewSigner)
	vc := NewViewChange(fake.NewAddress(0), authority, fakeChangeSetFactory{})

	allowed := vc.Wait()
	require.True(t, allowed)

	vc = NewViewChange(fake.NewAddress(1), authority, fakeChangeSetFactory{})
	allowed = vc.Wait()
	require.False(t, allowed)
}

func TestConstant_Verify(t *testing.T) {
	vc := NewViewChange(fake.NewAddress(0), fake.NewAuthority(3, fake.NewSigner), fakeChangeSetFactory{})

	curr, err := vc.Verify(fake.NewAddress(0), 0)
	require.NoError(t, err)
	require.Equal(t, 3, curr.Len())

	_, err = vc.Verify(fake.NewAddress(1), 0)
	require.EqualError(t, err, "<fake.Address[1]> is not the leader")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeChangeSetFactory struct {
	viewchange.ChangeSetFactory
}
