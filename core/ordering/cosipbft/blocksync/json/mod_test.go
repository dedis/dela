package json

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/ordering/cosipbft/blocksync/types"
	otypes "go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func TestMsgFormat_Encode(t *testing.T) {
	format := msgFormat{}

	ctx := fake.NewContext()

	data, err := format.Encode(ctx, types.NewSyncMessage(5))
	require.NoError(t, err)
	require.Equal(t, `{"Message":{"LatestIndex":5}}`, string(data))

	data, err = format.Encode(ctx, types.NewSyncRequest(3))
	require.NoError(t, err)
	require.Equal(t, `{"Request":{"From":3}}`, string(data))

	data, err = format.Encode(ctx, types.NewSyncReply(fakeLink{}))
	require.NoError(t, err)
	require.Equal(t, `{"Reply":{"Link":{}}}`, string(data))

	data, err = format.Encode(ctx, types.NewSyncAck())
	require.NoError(t, err)
	require.Equal(t, `{"Ack":{}}`, string(data))

	_, err = format.Encode(ctx, fake.Message{})
	require.EqualError(t, err, "unsupported message 'fake.Message'")

	_, err = format.Encode(ctx, types.NewSyncReply(fakeLink{err: xerrors.New("oops")}))
	require.EqualError(t, err, "link serialization failed: oops")

	_, err = format.Encode(fake.NewBadContext(), types.NewSyncAck())
	require.EqualError(t, err, "marshal failed: fake error")
}

func TestMsgFormat_Decode(t *testing.T) {
	format := msgFormat{}

	ctx := fake.NewContext()
	ctx = serde.WithFactory(ctx, types.LinkKey{}, fakeLinkFac{})

	msg, err := format.Decode(ctx, []byte(`{"Message":{}}`))
	require.NoError(t, err)
	require.Equal(t, types.NewSyncMessage(0), msg)

	msg, err = format.Decode(ctx, []byte(`{"Request":{}}`))
	require.NoError(t, err)
	require.Equal(t, types.NewSyncRequest(0), msg)

	msg, err = format.Decode(ctx, []byte(`{"Reply":{"Link":{}}}`))
	require.NoError(t, err)
	require.Equal(t, types.NewSyncReply(fakeLink{}), msg)

	msg, err = format.Decode(ctx, []byte(`{"Ack":{}}`))
	require.NoError(t, err)
	require.Equal(t, types.NewSyncAck(), msg)

	_, err = format.Decode(ctx, []byte(`{}`))
	require.EqualError(t, err, "message is empty")

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, "unmarshal failed: fake error")

	ctx = serde.WithFactory(ctx, types.LinkKey{}, fakeLinkFac{err: xerrors.New("oops")})
	_, err = format.Decode(ctx, []byte(`{"Reply":{"Link":{}}}`))
	require.EqualError(t, err, "couldn't decode link: oops")

	ctx = serde.WithFactory(ctx, types.LinkKey{}, fake.MessageFactory{})
	_, err = format.Decode(ctx, []byte(`{"Reply":{"Link":{}}}`))
	require.EqualError(t, err, "invalid link factory 'fake.MessageFactory'")
}

// Utility functions -----------------------------------------------------------

type fakeLink struct {
	otypes.BlockLink

	err error
}

func (link fakeLink) Serialize(serde.Context) ([]byte, error) {
	return []byte("{}"), link.err
}

type fakeLinkFac struct {
	otypes.BlockLinkFactory

	err error
}

func (fac fakeLinkFac) BlockLinkOf(serde.Context, []byte) (otypes.BlockLink, error) {
	return fakeLink{}, fac.err
}
