package json

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
)

func TestChainFormat_Encode(t *testing.T) {
	format := chainFormat{}

	ctx := fake.NewContext()

	data, err := format.Encode(ctx, types.NewChain(fakeLink{}, nil))
	require.NoError(t, err)
	require.Equal(t, `{"Links":[{}]}`, string(data))

	_, err = format.Encode(ctx, fake.Message{})
	require.EqualError(t, err, "unsupported message 'fake.Message'")

	_, err = format.Encode(ctx, types.NewChain(fakeLink{err: fake.GetError()}, nil))
	require.EqualError(t, err, fake.Err("couldn't serialize link"))

	_, err = format.Encode(fake.NewBadContext(), types.NewChain(fakeLink{}, nil))
	require.EqualError(t, err, fake.Err("failed to marshal"))
}

func TestChainFormat_Decode(t *testing.T) {
	format := chainFormat{}

	ctx := fake.NewContext()
	ctx = serde.WithFactory(ctx, types.LinkKey{}, fakeLinkFac{})

	chain, err := format.Decode(ctx, []byte(`{"Links":[{}, {}, {}]}`))
	require.NoError(t, err)
	require.Equal(t, types.NewChain(fakeLink{}, []types.Link{fakeLink{}, fakeLink{}}), chain)

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, fake.Err("failed to unmarshal"))

	_, err = format.Decode(ctx, []byte(`{}`))
	require.EqualError(t, err, "chain cannot be empty")

	badCtx := serde.WithFactory(ctx, types.LinkKey{}, fake.MessageFactory{})
	_, err = format.Decode(badCtx, []byte(`{"Links":[{}]}`))
	require.EqualError(t, err, "invalid link factory 'fake.MessageFactory'")

	badCtx = serde.WithFactory(ctx, types.LinkKey{}, fakeLinkFac{errLink: fake.GetError()})
	_, err = format.Decode(badCtx, []byte(`{"Links":[{}, {}]}`))
	require.EqualError(t, err, fake.Err("couldn't deserialize link"))

	badCtx = serde.WithFactory(ctx, types.LinkKey{}, fakeLinkFac{errBlockLink: fake.GetError()})
	_, err = format.Decode(badCtx, []byte(`{"Links": [{}]}`))
	require.EqualError(t, err, fake.Err("couldn't deserialize block link"))
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeLink struct {
	types.BlockLink

	err error
}

func (link fakeLink) Serialize(serde.Context) ([]byte, error) {
	return []byte(`{}`), link.err
}

type fakeLinkFac struct {
	types.LinkFactory

	errLink      error
	errBlockLink error
}

func (link fakeLinkFac) LinkOf(serde.Context, []byte) (types.Link, error) {
	return fakeLink{}, link.errLink
}

func (link fakeLinkFac) BlockLinkOf(serde.Context, []byte) (types.BlockLink, error) {
	return fakeLink{}, link.errBlockLink
}
