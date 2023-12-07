package json

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/ordering/cosipbft/fastsync/types"
	otypes "go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/testing/fake"
)

func TestMsgFormat_Encode(t *testing.T) {
	format := msgFormat{}

	ctx := fake.NewContext()

	data, err := format.Encode(ctx, types.NewCatchupMessage(false, []otypes.BlockLink{fakeLink{}}))
	require.NoError(t, err)
	require.Equal(t, `{"Catchup":{"SplitMessage":false,"BlockLinks":[{}]}}`, string(data))

	data, err = format.Encode(ctx, types.NewRequestCatchupMessage(1, 3))
	require.NoError(t, err)
	require.Equal(t, `{"Request":{"SplitMessageSize":1,"Latest":3}}`, string(data))

	_, err = format.Encode(ctx, fake.Message{})
	require.EqualError(t, err, "unsupported message 'fake.Message'")

	_, err = format.Encode(ctx,
		types.NewCatchupMessage(false, []otypes.BlockLink{fakeLink{err: fake.GetError()}}))
	require.EqualError(t, err, fake.Err("failed to encode blocklink"))
}

func TestMsgFormat_Decode(t *testing.T) {
	format := msgFormat{}

	ctx := fake.NewContext()
	ctx = serde.WithFactory(ctx, types.LinkKey{}, fakeLinkFac{})

	msg, err := format.Decode(ctx, []byte(`{"Catchup":{"SplitMessage":true,"BlockLinks":[{}]}}`))
	require.NoError(t, err)
	require.Equal(t, types.NewCatchupMessage(true, []otypes.BlockLink{fakeLink{}}), msg)

	msg, err = format.Decode(ctx, []byte(`{"Request":{"SplitMessageSize":1,"Latest":3}}`))
	require.NoError(t, err)
	require.Equal(t, types.NewRequestCatchupMessage(1, 3), msg)

	_, err = format.Decode(ctx, []byte(`{}`))
	require.EqualError(t, err, "message is empty")

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, fake.Err("unmarshal failed"))
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeLink struct {
	otypes.BlockLink

	err error
}

func (link fakeLink) Serialize(serde.Context) ([]byte, error) {
	return []byte("{}"), link.err
}

type fakeLinkFac struct {
	otypes.LinkFactory

	err error
}

func (fac fakeLinkFac) BlockLinkOf(serde.Context, []byte) (otypes.BlockLink, error) {
	return fakeLink{}, fac.err
}
