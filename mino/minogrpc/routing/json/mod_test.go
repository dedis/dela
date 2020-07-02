package json

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino/minogrpc/routing"
	"go.dedis.ch/dela/serde"
)

func TestFormat_Encode(t *testing.T) {
	authority := fake.NewAuthority(1, fake.NewSigner)

	treeRouting, err := routing.NewTreeRouting(authority)
	require.NoError(t, err)

	format := rtingFormat{}
	ctx := serde.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, treeRouting)
	require.NoError(t, err)
	require.Regexp(t, `{"Root":0,"Addresses":\["[^"]+"\]}`, string(data))

	_, err = format.Encode(ctx, nil)
	require.EqualError(t, err, "found '<nil>' but expected 'routing.TreeRouting'")

	_, err = format.Encode(fake.NewBadContext(), treeRouting)
	require.EqualError(t, err, "couldn't marshal to JSON: fake error")
}

func TestFormat_Decode(t *testing.T) {
	format := rtingFormat{}

	ctx := serde.NewContext(fake.ContextEngine{})
	ctx = serde.WithFactory(ctx, routing.AddrKey{}, fake.AddressFactory{})

	rting, err := format.Decode(ctx, []byte(`{"Addresses":[[]]}`))
	require.NoError(t, err)
	require.IsType(t, routing.TreeRouting{}, rting)

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, "couldn't deserialize message: fake error")

	badCtx := serde.WithFactory(ctx, routing.AddrKey{}, nil)
	_, err = format.Decode(badCtx, []byte(`{}`))
	require.EqualError(t, err, "found factory '<nil>'")

	_, err = format.Decode(ctx, []byte(`{}`))
	require.EqualError(t, err, "couldn't create tree routing: invalid root index 0")
}
