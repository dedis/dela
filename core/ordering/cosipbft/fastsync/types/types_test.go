package types

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/validation/simple"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/testing/fake"
)

var testCalls = &fake.Call{}

func init() {
	RegisterMessageFormat(fake.GoodFormat,
		fake.Format{Msg: CatchupMessage{}, Call: testCalls})
	RegisterMessageFormat(fake.BadFormat, fake.NewBadFormat())
}

func TestRequestCatchupMessage_GetChain(t *testing.T) {
	m := NewRequestCatchupMessage(1, 42)

	require.Equal(t, uint64(42), m.GetLatest())
	require.Equal(t, uint64(1), m.GetSplitMessageSize())
}

func TestRequestCatchupMessage_Serialize(t *testing.T) {
	m := NewRequestCatchupMessage(1, 42)

	data, err := m.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, fake.GetFakeFormatValue(), data)

	_, err = m.Serialize(fake.NewBadContext())
	require.EqualError(t, err, fake.Err("encoding failed"))
}

func TestCatchupMessage_GetBlockLinks(t *testing.T) {
	m := NewCatchupMessage(false, makeChain(t, 0, 2))

	require.Equal(t, 2, len(m.GetBlockLinks()))
	require.Equal(t, false, m.GetSplitMessage())
}

func TestCatchupMessage_Serialize(t *testing.T) {
	m := NewCatchupMessage(false, makeChain(t, 0, 2))

	data, err := m.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, fake.GetFakeFormatValue(), data)

	_, err = m.Serialize(fake.NewBadContext())
	require.EqualError(t, err, fake.Err("encoding failed"))
}

func TestMessageFactory_Deserialize(t *testing.T) {
	testCalls.Clear()

	linkFac := types.NewLinkFactory(nil, nil, nil)

	fac := NewMessageFactory(linkFac)

	msg, err := fac.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, CatchupMessage{}, msg)

	factory := testCalls.Get(0, 0).(serde.Context).GetFactory(LinkKey{})
	require.NotNil(t, factory)

	_, err = fac.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, fake.Err("decoding failed"))
}

// -----------------------------------------------------------------------------
// Utility functions

func makeChain(t *testing.T, start, count uint64) []types.BlockLink {
	blocks := make([]types.BlockLink, count)

	for index := uint64(0); index < count; index++ {
		block, err := types.NewBlock(simple.NewResult(nil), types.WithIndex(index))
		require.NoError(t, err)

		blocks[index-start], err = types.NewBlockLink(types.Digest{}, block)
		require.NoError(t, err)
	}

	return blocks
}
