package types

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
)

func init() {
	RegisterHandshakeFormat(fake.GoodFormat, fake.Format{Msg: Handshake{}})
	RegisterHandshakeFormat(fake.BadFormat, fake.NewBadFormat())
	RegisterHandshakeFormat(fake.MsgFormat, fake.NewMsgFormat())
}

func TestHandshake_GetHeight(t *testing.T) {
	hs := NewHandshake(3, nil)

	require.Equal(t, 3, hs.GetHeight())
}

func TestHandshake_GetAddresses(t *testing.T) {
	hs := NewHandshake(3, makeAddrs(5)...)

	require.Len(t, hs.GetAddresses(), 5)
}

func TestHandshake_Serialize(t *testing.T) {
	hs := NewHandshake(3, makeAddrs(5)...)

	ctx := fake.NewContext()

	data, err := hs.Serialize(ctx)
	require.NoError(t, err)
	require.Equal(t, fake.GetFakeFormatValue(), data)

	_, err = hs.Serialize(fake.NewBadContext())
	require.EqualError(t, err, fake.Err("encode"))
}

func TestHandshakeFactory_Deserialize(t *testing.T) {
	fac := NewHandshakeFactory(fake.AddressFactory{})

	msg, err := fac.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, Handshake{}, msg)

	_, err = fac.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, fake.Err("decode"))

	_, err = fac.Deserialize(fake.NewMsgContext(), nil)
	require.EqualError(t, err, "invalid handshake 'fake.Message'")
}

// -----------------------------------------------------------------------------
// Utility functions

func makeAddrs(n int) []mino.Address {
	addrs := make([]mino.Address, n)
	for i := range addrs {
		addrs[i] = fake.NewAddress(i)
	}

	return addrs
}
