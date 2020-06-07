package constant

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestConstant_Wait(t *testing.T) {
	authority := fake.NewAuthority(3, fake.NewSigner)
	vc := NewViewChange(fake.NewAddress(0), authority)

	allowed := vc.Wait()
	require.True(t, allowed)

	vc = NewViewChange(fake.NewAddress(1), authority)
	allowed = vc.Wait()
	require.False(t, allowed)
}

func TestConstant_Verify(t *testing.T) {
	vc := NewViewChange(fake.NewAddress(0), fake.NewAuthority(3, fake.NewSigner))

	prev, curr, err := vc.Verify(fake.NewAddress(0))
	require.NoError(t, err)
	require.Equal(t, 3, prev.Len())
	require.Equal(t, 3, curr.Len())
}
