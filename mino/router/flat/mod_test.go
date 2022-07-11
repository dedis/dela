package flat

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/router/flat/types"
)

func TestRouter_New(t *testing.T) {
	router := NewRouter(fake.AddressFactory{})
	addrs := makeAddrs(10)

	_, err := router.New(mino.NewAddresses(addrs...), addrs[0])
	require.NoError(t, err)
}

func TestRouter_GetPacketFactory(t *testing.T) {
	router := NewRouter(fake.AddressFactory{})

	require.NotNil(t, router.GetPacketFactory())
}

func TestRouter_GetHandshakeFactory(t *testing.T) {
	router := NewRouter(fake.AddressFactory{})

	require.NotNil(t, router.GetHandshakeFactory())
}

func TestRouter_GenerateTableFrom(t *testing.T) {
	router := NewRouter(fake.AddressFactory{})

	hs := types.NewHandshake(makeAddrs(5)...)
	table, err := router.GenerateTableFrom(hs)
	require.NoError(t, err)
	require.NotNil(t, table)
}

func TestTable_Make(t *testing.T) {
	table := NewTable(makeAddrs(5))

	pkt := table.Make(fake.NewAddress(0), makeAddrs(3), []byte{1, 2, 3})
	require.NotNil(t, pkt)
	require.Equal(t, fake.NewAddress(0), pkt.GetSource())
	require.Len(t, pkt.GetDestination(), 3)
	require.Equal(t, []byte{1, 2, 3}, pkt.GetMessage())
}

func TestTable_PrepareHandshakeFor(t *testing.T) {
	addrs := makeAddrs(5)
	table := NewTable(addrs)

	hs := table.PrepareHandshakeFor(fake.NewAddress(1))
	require.Equal(t, addrs, hs.(types.Handshake).GetKnown())
}

func TestTable_Forward(t *testing.T) {
	table := NewTable(makeAddrs(20))

	pkt := types.NewPacket("", fake.NewAddress(0), []byte{1, 2, 3}, makeAddrs(20)...)

	routes, voids := table.Forward(pkt)
	require.Len(t, voids, 0)
	require.Len(t, routes, 20)
}

func TestTable_OnFailure(t *testing.T) {
	table := NewTable(makeAddrs(5))
	err := table.OnFailure(fake.NewAddress(3))
	require.EqualError(t, err, "flat routing has no alternatives")
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
