package tree

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/mino/router/tree/types"
)

func TestRouter_MakePacket(t *testing.T) {
	r := Router{}
	p := r.MakePacket(fake.NewAddress(0), []mino.Address{fake.NewAddress(1)}, []byte("hi"))
	packet, ok := p.(*types.Packet)
	require.True(t, ok)
	require.Equal(t, fake.NewAddress(0), packet.Source)
	require.Len(t, packet.Dest, 1)
	require.Equal(t, fake.NewAddress(1), packet.Dest[0])
	require.Equal(t, "hi", string(packet.Message))
}

func TestRouter_Forward(t *testing.T) {

	r := NewRouter(2, fake.AddressFactory{})
	r.newTree(2)

	_, err := r.Forward(nil, nil)
	require.EqualError(t, err, "expected to have *types.Packet, got: <nil>")

	n := 7
	addrs := make([]mino.Address, 7)
	for i := 0; i < n; i++ {
		addrs[i] = fake.NewAddress(i)
	}

	packet := makePacket(nil, addrs[0], addrs...)

	_, err = r.Forward(newFakeMemship(addrs[0]), packet)
	require.NoError(t, err)

	// r.root.Display(os.Stdout)
	// Node[fake.Address[0]-index[0]-lastIndex[6]](
	// 	Node[fake.Address[1]-index[1]-lastIndex[3]](
	// 		Node[fake.Address[2]-index[2]-lastIndex[2]]()
	// 		Node[fake.Address[3]-index[3]-lastIndex[3]]()
	// 	)
	// 	Node[fake.Address[4]-index[4]-lastIndex[6]](
	// 		Node[fake.Address[5]-index[5]-lastIndex[5]]()
	// 		Node[fake.Address[6]-index[6]-lastIndex[6]]()
	// 	)
	// )

	type test struct {
		from        mino.Address
		destination []mino.Address
		expected    map[mino.Address]router.Packet
	}

	table := []test{
		{
			// If the source and the destination is unknown, and the source is
			// not one of my children, then the source will be added as the
			// root, and the tree will be recomputed as follow:
			// <nil>-index[0]-lastIndex[7]](
			// 	Node[fake.Address[1]-index[1]-lastIndex[3]](
			// 		Node[fake.Address[2]-index[2]-lastIndex[2]]()
			// 		Node[fake.Address[3]-index[3]-lastIndex[3]]()
			// 	)
			// 	Node[fake.Address[4]-index[4]-lastIndex[7]](
			// 		Node[fake.Address[5]-index[5]-lastIndex[5]]()
			// 		Node[fake.Address[6]-index[6]-lastIndex[6]]()
			// 		Node[fake.Address[110]-index[7]-lastIndex[7]]()
			// 	)
			// )
			from:        fake.NewAddress(101),
			destination: []mino.Address{fake.NewAddress(110)},
			expected: map[mino.Address]router.Packet{
				fake.NewAddress(4): makePacket(nil, fake.NewAddress(101), fake.NewAddress(110)),
			},
		},
		{
			// The source is one of the parent
			from:        fake.NewAddress(102),
			destination: []mino.Address{fake.NewAddress(0), fake.NewAddress(1), fake.NewAddress(2), fake.NewAddress(3), fake.NewAddress(4), fake.NewAddress(110)},
			expected: map[mino.Address]router.Packet{
				fake.NewAddress(0): makePacket(nil, fake.NewAddress(102), fake.NewAddress(0)),
				fake.NewAddress(1): makePacket(nil, fake.NewAddress(102), fake.NewAddress(1), fake.NewAddress(2), fake.NewAddress(3)),
				fake.NewAddress(4): makePacket(nil, fake.NewAddress(102), fake.NewAddress(4), fake.NewAddress(110)),
			},
		},
		{
			// The source is one of the children, but the destination is
			// unknown. In that case we send to nil, as we don't want a children
			// to say who we should connect to.
			from: fake.NewAddress(2),
			destination: []mino.Address{
				fake.NewAddress(111),
			},
			expected: map[mino.Address]router.Packet{
				nil: makePacket(nil, fake.NewAddress(2), fake.NewAddress(111)),
			},
		},
	}

	for _, entry := range table {
		packet := makePacket(nil, entry.from, entry.destination...)
		require.NoError(t, err)
		result, err := r.Forward(newFakeMemship(entry.from), packet)

		require.NoError(t, err)
		compareMap(t, entry.expected, result)
	}
}

// -----------------------------------------------------------------------------
// Utility functions

func makePacket(msg []byte, me mino.Address, dest ...mino.Address) router.Packet {
	return &types.Packet{
		Source:  me,
		Dest:    dest,
		Message: msg,
	}
}

func compareMap(t *testing.T, expected, result map[mino.Address]router.Packet) {
	require.Len(t, result, len(expected))

	for k, v := range expected {
		r, found := result[k]
		require.True(t, found, "entry with key '%s' not found in result", k)
		require.Equal(t, v.GetSource(), r.GetSource())
		require.Equal(t, v.GetDestination(), r.GetDestination(), "%v != %v, key=%s", v.GetDestination(), r.GetDestination(), k)
	}
}

func newFakeMemship(me mino.Address, addrs ...mino.Address) router.Membership {
	return fakeMemship{
		me:    me,
		addrs: addrs,
	}
}

type fakeMemship struct {
	me    mino.Address
	addrs []mino.Address
}

func (m fakeMemship) GetLocal() mino.Address {
	return m.me
}
func (m fakeMemship) GetAddresses() []mino.Address {
	return m.addrs
}
