package types

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/validation/simple"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
)

var testCalls = &fake.Call{}

func init() {
	RegisterMessageFormat(fake.GoodFormat, fake.Format{Msg: SyncMessage{}, Call: testCalls})
	RegisterMessageFormat(fake.BadFormat, fake.NewBadFormat())
}

func TestSyncMessage_GetChain(t *testing.T) {
	m := NewSyncMessage(makeChain(t, 5))

	require.NotNil(t, m.GetChain())
}

func TestSyncMessage_GetLatestIndex(t *testing.T) {
	m := NewSyncMessage(makeChain(t, 5))

	require.Equal(t, uint64(5), m.GetLatestIndex())
}

func TestSyncMessage_Serialize(t *testing.T) {
	m := NewSyncMessage(makeChain(t, 6))

	data, err := m.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, fake.GetFakeFormatValue(), data)

	_, err = m.Serialize(fake.NewBadContext())
	require.EqualError(t, err, fake.Err("encoding failed"))
}

func TestSyncRequest_GetFrom(t *testing.T) {
	m := NewSyncRequest(2)

	require.Equal(t, uint64(2), m.GetFrom())
}

func TestSyncRequest_Serialize(t *testing.T) {
	m := NewSyncRequest(3)

	data, err := m.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, fake.GetFakeFormatValue(), data)

	_, err = m.Serialize(fake.NewBadContext())
	require.EqualError(t, err, fake.Err("encoding failed"))
}

func TestSyncReply_GetLink(t *testing.T) {
	link, err := types.NewBlockLink(types.Digest{1}, types.Block{})
	require.NoError(t, err)

	m := NewSyncReply(link)

	require.Equal(t, link, m.GetLink())
}

func TestSyncReply_Serialize(t *testing.T) {
	link, err := types.NewBlockLink(types.Digest{}, types.Block{})
	require.NoError(t, err)

	m := NewSyncReply(link)

	data, err := m.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, fake.GetFakeFormatValue(), data)

	_, err = m.Serialize(fake.NewBadContext())
	require.EqualError(t, err, fake.Err("encoding failed"))
}

func TestSyncAck_Serialize(t *testing.T) {
	m := NewSyncAck()

	data, err := m.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, fake.GetFakeFormatValue(), data)

	_, err = m.Serialize(fake.NewBadContext())
	require.EqualError(t, err, fake.Err("encoding failed"))
}

func TestMessageFactory_Deserialize(t *testing.T) {
	testCalls.Clear()

	linkFac := types.NewLinkFactory(nil, nil, nil)

	fac := NewMessageFactory(linkFac, types.NewChainFactory(linkFac))

	msg, err := fac.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, SyncMessage{}, msg)

	factory := testCalls.Get(0, 0).(serde.Context).GetFactory(LinkKey{})
	require.NotNil(t, factory)

	_, err = fac.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, fake.Err("decoding failed"))
}

// -----------------------------------------------------------------------------
// Utility functions

func makeChain(t *testing.T, index uint64) types.Chain {
	block, err := types.NewBlock(simple.NewResult(nil), types.WithIndex(index))
	require.NoError(t, err)

	link, err := types.NewBlockLink(types.Digest{}, block)
	require.NoError(t, err)

	return types.NewChain(link, nil)
}
