package json

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino/minogrpc/routing"
	"go.dedis.ch/dela/serdeng"
)

func TestFormat_Decode(t *testing.T) {
	format := format{}

	ctx := serdeng.NewContext(fake.ContextEngine{})
	ctx = serdeng.WithFactory(ctx, routing.AddrKey{}, fake.AddressFactory{})

	rting, err := format.Decode(ctx, []byte(`{"Addresses":[[]]}`))
	require.NoError(t, err)
	require.IsType(t, routing.TreeRouting{}, rting)

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, "couldn't deserialize message: fake error")
}

func TestFormat_Encode(t *testing.T) {
	authority := fake.NewAuthority(1, fake.NewSigner)

	treeRouting, err := routing.NewTreeRouting(authority)
	require.NoError(t, err)

	format := format{}
	ctx := serdeng.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, treeRouting)
	require.NoError(t, err)
	require.Regexp(t, `{"Root":0,"Addresses":\["[^"]+"\]}`, string(data))
}
