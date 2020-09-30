package json

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/access/darc/types"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
)

const testValue = `{"Expressions":{"test":{"Identities":[{}],"Matches":[[0]]}}}`

func TestPermFormat_Encode(t *testing.T) {
	fmt := permFormat{}

	ctx := fake.NewContext()

	perm := types.NewPermission(types.WithRule("test", fake.PublicKey{}))

	data, err := fmt.Encode(ctx, perm)
	require.NoError(t, err)
	require.Equal(t, testValue, string(data))

	_, err = fmt.Encode(ctx, fake.Message{})
	require.EqualError(t, err, "invalid permission 'fake.Message'")

	_, err = fmt.Encode(fake.NewBadContext(), perm)
	require.EqualError(t, err, fake.Err("failed to marshal"))

	perm = types.NewPermission(types.WithRule("test", fake.NewBadPublicKey()))
	_, err = fmt.Encode(ctx, perm)
	require.EqualError(t, err, fake.Err("failed to encode expression: failed to serialize identity"))
}

func TestPermFormat_Decode(t *testing.T) {
	fmt := permFormat{}

	ctx := fake.NewContext()
	ctx = serde.WithFactory(ctx, types.PublicKeyFac{}, fake.PublicKeyFactory{})

	msg, err := fmt.Decode(ctx, []byte(testValue))
	require.NoError(t, err)
	require.Equal(t, types.NewPermission(types.WithRule("test", fake.PublicKey{})), msg)

	_, err = fmt.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, fake.Err("failed to unmarshal"))

	badCtx := serde.WithFactory(ctx, types.PublicKeyFac{}, nil)
	_, err = fmt.Decode(badCtx, []byte(testValue))
	require.EqualError(t, err, "failed to decode expression: invalid public key factory '<nil>'")

	badCtx = serde.WithFactory(ctx, types.PublicKeyFac{}, fake.NewBadPublicKeyFactory())
	_, err = fmt.Decode(badCtx, []byte(testValue))
	require.EqualError(t, err, fake.Err("failed to decode expression: public key"))
}
