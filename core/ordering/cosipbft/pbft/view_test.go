package pbft

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestView_Getters(t *testing.T) {
	param := ViewParam{
		From:   fake.NewAddress(0),
		ID:     types.Digest{1},
		Leader: 5,
	}

	view := NewView(param, fake.Signature{})

	require.Equal(t, fake.NewAddress(0), view.GetFrom())
	require.Equal(t, types.Digest{1}, view.GetID())
	require.Equal(t, uint16(5), view.GetLeader())
	require.Equal(t, fake.Signature{}, view.GetSignature())
}

func TestView_Verify(t *testing.T) {
	param := ViewParam{
		From:   fake.NewAddress(0),
		ID:     types.Digest{2},
		Leader: 3,
	}

	signer := bls.NewSigner()

	view, err := NewViewAndSign(param, signer)
	require.NoError(t, err)
	require.NoError(t, view.Verify(signer.GetPublicKey()))

	_, err = NewViewAndSign(param, fake.NewBadSigner())
	require.EqualError(t, err, fake.Err("signer"))

	err = view.Verify(fake.NewBadPublicKey())
	require.EqualError(t, err, fake.Err("verify"))
}
