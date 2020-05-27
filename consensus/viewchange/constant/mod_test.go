package constant

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestConstant_Wait(t *testing.T) {
	vc := NewViewChange(fake.NewAddress(0))
	ca := fake.NewAuthority(3, fake.NewSigner)

	rotate, allowed := vc.Wait(nil, ca)
	require.True(t, allowed)
	require.Equal(t, uint32(0), rotate)

	ca = fake.NewAuthorityWithBase(1, 3, fake.NewSigner)
	rotate, allowed = vc.Wait(nil, ca)
	require.False(t, allowed)
	require.Equal(t, uint32(0), rotate)
}

func TestConstant_Verify(t *testing.T) {
	vc := NewViewChange(fake.NewAddress(0))

	require.Equal(t, uint32(0), vc.Verify(nil, nil))
}
