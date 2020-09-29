package json

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cosi/threshold/types"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
)

func TestFormat_Encode(t *testing.T) {
	format := sigFormat{}
	sig := types.NewSignature(fake.Signature{}, []byte{0xab})

	ctx := fake.NewContext()

	data, err := format.Encode(ctx, sig)
	require.NoError(t, err)
	require.Equal(t, `{"Mask":"qw==","Aggregate":{}}`, string(data))

	_, err = format.Encode(ctx, fake.Message{})
	require.EqualError(t, err, "unsupported message of type 'fake.Message'")

	_, err = format.Encode(fake.NewBadContext(), sig)
	require.EqualError(t, err, fake.Err("couldn't marshal"))

	sig = types.NewSignature(fake.NewBadSignature(), nil)
	_, err = format.Encode(ctx, sig)
	require.EqualError(t, err, fake.Err("couldn't serialize aggregate"))
}

func TestFormat_Decode(t *testing.T) {
	format := sigFormat{}

	ctx := fake.NewContext()
	ctx = serde.WithFactory(ctx, types.AggKey{}, fake.SignatureFactory{})

	sig, err := format.Decode(ctx, []byte(`{"Mask":[1],"Aggregate":{}}`))
	require.NoError(t, err)
	require.Equal(t, []byte{1}, sig.(*types.Signature).GetMask())

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, fake.Err("couldn't unmarshal message"))

	ctx = serde.WithFactory(ctx, types.AggKey{}, fake.NewBadSignatureFactory())
	_, err = format.Decode(ctx, []byte(`{}`))
	require.EqualError(t, err, fake.Err("couldn't deserialize signature"))

	ctx = serde.WithFactory(ctx, types.AggKey{}, nil)
	_, err = format.Decode(ctx, []byte(`{}`))
	require.EqualError(t, err, "invalid factory of type '<nil>'")
}
