package types

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
)

func init() {
	RegisterPacketFormat(fake.GoodFormat, fake.Format{Msg: &Packet{}})
	RegisterPacketFormat(fake.BadFormat, fake.NewBadFormat())
	RegisterPacketFormat(fake.MsgFormat, fake.NewMsgFormat())
}

func TestPacket_GetSource(t *testing.T) {
	pkt := NewPacket(fake.NewAddress(0), nil)

	require.Equal(t, fake.NewAddress(0), pkt.GetSource())
}

func TestPacket_GetDestination(t *testing.T) {
	pkt := NewPacket(fake.NewAddress(0), nil, makeAddrs(5)...)

	require.Len(t, pkt.GetDestination(), 5)
}

func TestPacket_GetMessage(t *testing.T) {
	pkt := NewPacket(fake.NewAddress(0), []byte{1, 2, 3})

	require.Equal(t, []byte{1, 2, 3}, pkt.GetMessage())
}

func TestPacket_Add(t *testing.T) {
	pkt := NewPacket(fake.NewAddress(0), nil)
	require.Len(t, pkt.dest, 0)

	pkt.Add(fake.NewAddress(0))
	require.Len(t, pkt.dest, 1)

	pkt.Add(fake.NewAddress(0))
	require.Len(t, pkt.dest, 1)

	pkt.Add(fake.NewAddress(1))
	require.Len(t, pkt.dest, 2)
}

func TestPacket_Slice(t *testing.T) {
	pkt := NewPacket(fake.NewAddress(0), []byte{0xaa}, makeAddrs(10)...)

	newPkt := pkt.Slice(fake.NewAddress(500))
	require.Nil(t, newPkt)

	newPkt = pkt.Slice(fake.NewAddress(6))
	require.Equal(t, []mino.Address{fake.NewAddress(6)}, newPkt.GetDestination())
	require.Len(t, pkt.dest, 9)
}

func TestPacket_Serialize(t *testing.T) {
	pkt := NewPacket(fake.NewAddress(0), nil)

	data, err := pkt.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, fake.GetFakeFormatValue(), data)

	_, err = pkt.Serialize(fake.NewBadContext())
	require.EqualError(t, err, fake.Err("packet format"))
}

func TestPacketFactory_Deserialize(t *testing.T) {
	fac := NewPacketFactory(fake.AddressFactory{})

	msg, err := fac.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, &Packet{}, msg)

	_, err = fac.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, fake.Err("packet format"))

	_, err = fac.Deserialize(fake.NewMsgContext(), nil)
	require.EqualError(t, err, "invalid packet 'fake.Message'")
}
