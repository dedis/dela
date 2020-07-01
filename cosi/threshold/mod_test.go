package threshold

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minoch"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func TestCoSi_Basic(t *testing.T) {
	manager := minoch.NewManager()

	m1, err := minoch.NewMinoch(manager, "A")
	require.NoError(t, err)

	m2, err := minoch.NewMinoch(manager, "B")
	require.NoError(t, err)

	ca := fake.NewAuthorityFromMino(bls.NewSigner, m1, m2)
	c1 := NewCoSi(m1, ca.GetSigner(0))
	c1.Threshold = func(n int) int { return n - 1 }

	actor, err := c1.Listen(fakeReactor{})
	require.NoError(t, err)

	c2 := NewCoSi(m2, ca.GetSigner(1))
	_, err = c2.Listen(fakeReactor{err: xerrors.New("oops")})
	require.NoError(t, err)

	ctx := context.Background()
	sig, err := actor.Sign(ctx, fake.Message{}, ca)
	require.NoError(t, err)
	require.NotNil(t, sig)

	verifier, err := c1.GetVerifierFactory().FromAuthority(ca)
	require.NoError(t, err)
	require.NoError(t, verifier.Verify([]byte{0xff}, sig))
}

func TestDefaultThreshold(t *testing.T) {
	require.Equal(t, 2, defaultThreshold(2))
	require.Equal(t, 5, defaultThreshold(5))
}

func TestCoSi_GetSigner(t *testing.T) {
	c := &CoSi{signer: fake.NewSigner()}
	require.NotNil(t, c.GetSigner())
}

func TestCoSi_GetPublicKeyFactory(t *testing.T) {
	c := &CoSi{signer: fake.NewSigner()}
	require.NotNil(t, c.GetPublicKeyFactory())
}

func TestCoSi_GetSignatureFactory(t *testing.T) {
	c := &CoSi{signer: fake.NewSigner()}
	require.NotNil(t, c.GetSignatureFactory())
}

func TestCoSi_Listen(t *testing.T) {
	c := &CoSi{
		mino:   fake.Mino{},
		signer: fake.NewSigner(),
	}

	actor, err := c.Listen(fakeReactor{})
	require.NoError(t, err)
	require.NotNil(t, actor)

	c.mino = fake.NewBadMino()
	_, err = c.Listen(fakeReactor{})
	require.EqualError(t, err, "couldn't make rpc: fake error")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeReactor struct {
	fake.MessageFactory

	err error
}

func (h fakeReactor) Invoke(addr mino.Address, in serde.Message) ([]byte, error) {
	return []byte{0xff}, h.err
}
