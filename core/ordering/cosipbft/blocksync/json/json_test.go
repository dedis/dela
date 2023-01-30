package json

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/ordering/cosipbft/blocksync/types"
	otypes "go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
)

func TestMsgFormat_Encode(t *testing.T) {
	format := msgFormat{}

	ctx := fake.NewContext()

	data, err := format.Encode(ctx, types.NewSyncMessage(fakeChain{}))
	require.NoError(t, err)
	require.Equal(t, `{"Message":{"Chain":{}}}`, string(data))

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

	_, err = format.Encode(ctx, types.NewSyncMessage(fakeChain{err: fake.GetError()}))
	require.EqualError(t, err, fake.Err("failed to encode chain"))

	_, err = format.Encode(ctx, types.NewSyncReply(fakeLink{err: fake.GetError()}))
	require.EqualError(t, err, fake.Err("link serialization failed"))

	_, err = format.Encode(fake.NewBadContext(), types.NewSyncAck())
	require.EqualError(t, err, fake.Err("marshal failed"))
}

func TestMsgFormat_Decode(t *testing.T) {
	format := msgFormat{}

	ctx := fake.NewContext()
	ctx = serde.WithFactory(ctx, types.LinkKey{}, fakeLinkFac{})
	ctx = serde.WithFactory(ctx, types.ChainKey{}, fakeChainFac{})

	msg, err := format.Decode(ctx, []byte(`{"Message":{}}`))
	require.NoError(t, err)
	require.Equal(t, types.NewSyncMessage(fakeChain{}), msg)

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
	require.EqualError(t, err, fake.Err("unmarshal failed"))

	ctx = serde.WithFactory(ctx, types.ChainKey{}, fakeChainFac{err: fake.GetError()})
	_, err = format.Decode(ctx, []byte(`{"Message":{}}`))
	require.EqualError(t, err, fake.Err("failed to decode chain"))

	ctx = serde.WithFactory(ctx, types.ChainKey{}, fake.MessageFactory{})
	_, err = format.Decode(ctx, []byte(`{"Message":{}}`))
	require.EqualError(t, err, "invalid chain factory 'fake.MessageFactory'")

	ctx = serde.WithFactory(ctx, types.LinkKey{}, fakeLinkFac{err: fake.GetError()})
	_, err = format.Decode(ctx, []byte(`{"Reply":{"Link":{}}}`))
	require.EqualError(t, err, fake.Err("couldn't decode link"))

	ctx = serde.WithFactory(ctx, types.LinkKey{}, fake.MessageFactory{})
	_, err = format.Decode(ctx, []byte(`{"Reply":{"Link":{}}}`))
	require.EqualError(t, err, "invalid link factory 'fake.MessageFactory'")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeChain struct {
	otypes.Chain

	err error
}

func (chain fakeChain) Serialize(serde.Context) ([]byte, error) {
	return []byte("{}"), chain.err
}

type fakeChainFac struct {
	otypes.ChainFactory

	err error
}

func (fac fakeChainFac) ChainOf(serde.Context, []byte) (otypes.Chain, error) {
	return fakeChain{}, fac.err
}

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
