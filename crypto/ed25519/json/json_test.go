package json

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/crypto/ed25519"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/kyber/v3"
)

func TestPubKkeyFormat_Encode(t *testing.T) {
	format := pubkeyFormat{}
	signer := ed25519.NewSigner()

	msg := signer.GetPublicKey()

	ctx := serde.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, msg)
	require.NoError(t, err)
	require.Regexp(t, `{"Name":"CURVE-ED25519","Data":"[^"]+"}`, string(data))

	_, err = format.Encode(fake.NewBadContext(), msg)
	require.EqualError(t, err, fake.Err("couldn't marshal"))

	_, err = format.Encode(fake.NewBadContext(), ed25519.NewPublicKeyFromPoint(badPoint{}))
	require.EqualError(t, err, fake.Err("couldn't marshal point"))

	_, err = format.Encode(fake.NewBadContext(), fake.Message{})
	require.EqualError(t, err, "unsupported message of type 'fake.Message'")
}

func TestPubKkeyFormat_Decode(t *testing.T) {
	format := pubkeyFormat{}
	signer := ed25519.NewSigner()

	ctx := fake.NewContextWithFormat(serde.FormatJSON)

	data, err := signer.GetPublicKey().Serialize(ctx)
	require.NoError(t, err)

	pubkey, err := format.Decode(ctx, data)
	require.NoError(t, err)
	require.True(t, signer.GetPublicKey().Equal(pubkey.(ed25519.PublicKey)))

	_, err = format.Decode(ctx, []byte(`{"Data":[]}`))
	require.EqualError(t, err, "couldn't create public key: couldn't unmarshal point: invalid Ed25519 curve point")

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, fake.Err("couldn't unmarshal public key"))
}

func TestSigFormat_Encode(t *testing.T) {
	format := sigFormat{}
	ctx := fake.NewContext()

	signer := ed25519.NewSigner()
	sign, err := signer.Sign([]byte("hello"))
	require.NoError(t, err)

	data, err := format.Encode(ctx, sign)
	require.NoError(t, err)
	require.Regexp(t, `{"Name":"CURVE-ED25519","Data":"[^"]+"}`, string(data))

	_, err = format.Encode(fake.NewBadContext(), sign)
	require.EqualError(t, err, fake.Err("couldn't marshal"))

	_, err = format.Encode(ctx, fake.Message{})
	require.EqualError(t, err, "unsupported message of type 'fake.Message'")
}

func TestSigFormat_Decode(t *testing.T) {
	format := sigFormat{}
	ctx := fake.NewContextWithFormat(serde.FormatJSON)

	signer := ed25519.NewSigner()
	signatue, err := signer.Sign([]byte("hello"))
	require.NoError(t, err)

	data, err := signatue.Serialize(ctx)
	require.NoError(t, err)

	msg, err := format.Decode(ctx, data)
	require.NoError(t, err)
	require.True(t, signatue.Equal(msg.(ed25519.Signature)))

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, fake.Err("couldn't unmarshal signature"))
}

// -----------------------------------------------------------------------------
// Utility functions

type badPoint struct {
	kyber.Point
}

func (p badPoint) MarshalBinary() ([]byte, error) {
	return nil, fake.GetError()
}
