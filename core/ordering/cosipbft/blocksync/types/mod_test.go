package types

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
)

var testCalls = &fake.Call{}

func init() {
	RegisterMessageFormat(fake.GoodFormat, fake.Format{Msg: SyncMessage{}, Call: testCalls})
	RegisterMessageFormat(fake.BadFormat, fake.NewBadFormat())
}

func TestSyncMessage_GetLatestIndex(t *testing.T) {
	m := NewSyncMessage(5)

	require.Equal(t, uint64(5), m.GetLatestIndex())
}

func TestSyncMessage_Serialize(t *testing.T) {
	m := NewSyncMessage(6)

	data, err := m.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = m.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "encoding failed: fake error")
}

func TestSyncRequest_GetFrom(t *testing.T) {
	m := NewSyncRequest(2)

	require.Equal(t, uint64(2), m.GetFrom())
}

func TestSyncRequest_Serialize(t *testing.T) {
	m := NewSyncRequest(3)

	data, err := m.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = m.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "encoding failed: fake error")
}

func TestSyncReply_GetLink(t *testing.T) {
	link := types.NewBlockLink(types.Digest{1}, types.Block{}, nil, nil, nil)

	m := NewSyncReply(link)

	require.Equal(t, link, m.GetLink())
}

func TestSyncReply_Serialize(t *testing.T) {
	m := NewSyncReply(types.NewBlockLink(types.Digest{}, types.Block{}, nil, nil, nil))

	data, err := m.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = m.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "encoding failed: fake error")
}

func TestSyncAck_Serialize(t *testing.T) {
	m := NewSyncAck()

	data, err := m.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = m.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "encoding failed: fake error")
}

func TestMessageFactory_Deserialize(t *testing.T) {
	testCalls.Clear()

	fac := NewMessageFactory(types.NewBlockLinkFactory(nil, nil, nil))

	msg, err := fac.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, SyncMessage{}, msg)

	factory := testCalls.Get(0, 0).(serde.Context).GetFactory(LinkKey{})
	require.NotNil(t, factory)

	_, err = fac.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, "decoding failed: fake error")
}
