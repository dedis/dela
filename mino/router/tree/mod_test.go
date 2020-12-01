package tree

import (
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/router/tree/types"
)

func TestRouter_GetPacketFactory(t *testing.T) {
	router := NewRouter(fake.AddressFactory{})

	require.NotNil(t, router.GetPacketFactory())
}

func TestRouter_GetHandshakeFactory(t *testing.T) {
	router := NewRouter(fake.AddressFactory{})

	require.NotNil(t, router.GetHandshakeFactory())
}

func TestRouter_New(t *testing.T) {
	f := func(height, n uint8) bool {
		router := NewRouter(fake.AddressFactory{})
		router.maxHeight = int(height)

		fakeAddrs := makeAddrs(int(n))
		table, err := router.New(mino.NewAddresses(fakeAddrs...), fakeAddrs[0])
		require.NoError(t, err)

		return router.maxHeight == table.(Table).tree.GetMaxHeight()
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestRouter_GenerateTableFrom(t *testing.T) {
	router := NewRouter(fake.AddressFactory{})

	hs := types.NewHandshake(3, makeAddrs(5)...)
	table, err := router.GenerateTableFrom(hs)
	require.NoError(t, err)
	require.NotNil(t, table)
}

func TestTable_Make(t *testing.T) {
	table := NewTable(3, makeAddrs(5))

	pkt := table.Make(fake.NewAddress(0), makeAddrs(3), []byte{1, 2, 3})
	require.NotNil(t, pkt)
	require.Equal(t, fake.NewAddress(0), pkt.GetSource())
	require.Len(t, pkt.GetDestination(), 3)
	require.Equal(t, []byte{1, 2, 3}, pkt.GetMessage())
}

func TestTable_PrepareHandshakeFor(t *testing.T) {
	table := NewTable(3, makeAddrs(5))

	hs := table.PrepareHandshakeFor(fake.NewAddress(1))
	require.Equal(t, 2, hs.(types.Handshake).GetHeight())
}

func TestTable_Forward(t *testing.T) {
	table := NewTable(3, makeAddrs(20))

	pkt := types.NewPacket(fake.NewAddress(0), []byte{1, 2, 3}, makeAddrs(20)...)

	routes, voids := table.Forward(pkt)
	require.Empty(t, voids)
	require.Len(t, routes, 5)

	table.tree.(*dynTree).offline[fake.NewAddress(1)] = struct{}{}
	routes, voids = table.Forward(pkt)
	require.Len(t, voids, 1)
	require.Len(t, routes, 5)
}

func TestTable_OnFailure(t *testing.T) {
	table := NewTable(1, makeAddrs(5))
	err := table.OnFailure(fake.NewAddress(3))
	require.EqualError(t, err, "address is unreachable")

	table = NewTable(3, makeAddrs(20))
	err = table.OnFailure(fake.NewAddress(12))
	require.NoError(t, err)
	require.Len(t, table.tree.(*dynTree).offline, 1)
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
