package types

import (
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"testing"
)

var testCalls = &fake.Call{}

func init() {
	RegisterMessageFormat(fake.GoodFormat, fake.Format{Msg: StartShuffle{}, Call: testCalls})
	RegisterMessageFormat(fake.BadFormat, fake.NewBadFormat())
}

func TestStartShuffle_GetThreshold(t *testing.T) {
	startShuffle := NewStartShuffle(5, "", nil)

	require.Equal(t, 5, startShuffle.GetThreshold())
}

func TestStartShuffle_GetElectionId(t *testing.T) {
	startShuffle := NewStartShuffle(0, "dummyId", nil)

	require.Equal(t, "dummyId", startShuffle.GetElectionId())
}

func TestStartShuffle_GetAddresses(t *testing.T) {
	startShuffle := NewStartShuffle(0, "", []mino.Address{fake.NewAddress(0)})

	require.Len(t, startShuffle.GetAddresses(), 1)
}

func TestStartShuffle_Serialize(t *testing.T) {
	startShuffle := StartShuffle{}

	_, err := startShuffle.Serialize(fake.NewBadContext())
	require.EqualError(t, err, fake.Err("couldn't encode StartShuffle message"))

	data, err := startShuffle.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, fake.GetFakeFormatValue(), data)
}

func TestEndShuffle_Serialize(t *testing.T) {
	endShuffle := NewEndShuffle()

	_, err := endShuffle.Serialize(fake.NewBadContext())
	require.EqualError(t, err, fake.Err("couldn't encode EndShuffle message"))

	data, err := endShuffle.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, fake.GetFakeFormatValue(), data)
}

func TestMessageFactory(t *testing.T) {
	factory := NewMessageFactory(fake.AddressFactory{})

	testCalls.Clear()

	_, err := factory.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, fake.Err("couldn't decode message"))

	msg, err := factory.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, StartShuffle{}, msg)

	require.Equal(t, 1, testCalls.Len())
	ctx := testCalls.Get(0, 0).(serde.Context)
	require.Equal(t, fake.AddressFactory{}, ctx.GetFactory(AddrKey{}))
}
