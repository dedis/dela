package tree

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/mino/router/tree/types"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
)

func TestRouter_MakePacket(t *testing.T) {
	r := Router{}
	p := r.MakePacket(fake.NewAddress(0), []mino.Address{fake.NewAddress(1)}, []byte("hi"))
	packet, ok := p.(types.Packet)
	require.True(t, ok)
	require.Equal(t, fake.NewAddress(0), packet.Source)
	require.Len(t, packet.Dest, 1)
	require.Equal(t, fake.NewAddress(1), packet.Dest[0])
	require.Equal(t, "hi", string(packet.Message))
}

func TestRouter_Forward(t *testing.T) {
	context := json.NewContext()

	r := NewRouter(newFakeMemship(7), 2, fake.AddressFactory{})
	r.newTree(0, 2)

	_, err := r.Forward(nil, nil, fake.NewBadContext())
	require.EqualError(t, err, "failed to deserialize packet: couldn't decode message: format 'FakeBad' is not implemented")

	r.fac = dummyFac{}
	packet := makePacket(nil, fake.NewAddress(0), fake.NewAddress(1))
	data, err := packet.Serialize(context)
	require.NoError(t, err)

	_, err = r.Forward(nil, data, json.NewContext())
	require.EqualError(t, err, "expected to have types.Packet, got: fake.Message")

	// r.root.Display(os.Stdout)
	// Node[fake.Address[2]-index[0]-lastIndex[6]](
	// 	Node[fake.Address[4]-index[1]-lastIndex[3]](
	// 		Node[fake.Address[5]-index[2]-lastIndex[2]](
	// 		)
	// 		Node[fake.Address[0]-index[3]-lastIndex[3]](
	// 		)
	// 	)
	// 	Node[fake.Address[3]-index[4]-lastIndex[6]](
	// 		Node[fake.Address[1]-index[5]-lastIndex[5]](
	// 		)
	// 		Node[fake.Address[6]-index[6]-lastIndex[6]](
	// 		)
	// 	)
	// )

	type test struct {
		from        mino.Address
		destination []mino.Address
		expected    map[mino.Address]router.Packet
	}

	table := []test{
		{
			// If the source and the destination is unknown, then is uses nil in
			// the map.
			from:        fake.NewAddress(101),
			destination: []mino.Address{fake.NewAddress(102)},
			expected: map[mino.Address]router.Packet{
				nil: makePacket(nil, fake.NewAddress(101), fake.NewAddress(102)),
			},
		},
		{
			// The source is unknown, which can happen if this is the
			// orchestrator addr. In that case we send the packet to the root of
			// the tree. This is the first packet sent by the orchestrator.
			from:        fake.NewAddress(101),
			destination: []mino.Address{fake.NewAddress(0), fake.NewAddress(1), fake.NewAddress(2)},
			expected: map[mino.Address]router.Packet{
				fake.NewAddress(2): makePacket(nil, fake.NewAddress(101), fake.NewAddress(0), fake.NewAddress(1), fake.NewAddress(2)),
			},
		},
		{
			// The root sends to everyone, which should generate one packet for
			// the children in the left, one for the children in the right, and
			// one for itself.
			from: fake.NewAddress(2),
			destination: []mino.Address{
				fake.NewAddress(0), fake.NewAddress(1), fake.NewAddress(2),
				fake.NewAddress(3), fake.NewAddress(4), fake.NewAddress(5),
				fake.NewAddress(6),
			},
			expected: map[mino.Address]router.Packet{
				// first children
				fake.NewAddress(4): makePacket(nil, fake.NewAddress(2), fake.NewAddress(0), fake.NewAddress(4), fake.NewAddress(5)),
				// second children
				fake.NewAddress(3): makePacket(nil, fake.NewAddress(2), fake.NewAddress(1), fake.NewAddress(3), fake.NewAddress(6)),
				// itself
				fake.NewAddress(2): makePacket(nil, fake.NewAddress(2), fake.NewAddress(2)),
			},
		},
		{
			// Node 3 sends a message to everyone. So it should generate 2
			// packets for each of its children, one packet for itself, and one
			// packet for its parent. The unknown packet is sent to the parent.
			from: fake.NewAddress(3),
			destination: []mino.Address{
				fake.NewAddress(0), fake.NewAddress(1), fake.NewAddress(2),
				fake.NewAddress(3), fake.NewAddress(4), fake.NewAddress(5),
				fake.NewAddress(6), fake.NewAddress(999),
			},
			expected: map[mino.Address]router.Packet{
				// first children
				fake.NewAddress(1): makePacket(nil, fake.NewAddress(3), fake.NewAddress(1)),
				// second children
				fake.NewAddress(6): makePacket(nil, fake.NewAddress(3), fake.NewAddress(6)),
				// itself
				fake.NewAddress(3): makePacket(nil, fake.NewAddress(3), fake.NewAddress(3)),
				// the parent
				fake.NewAddress(2): makePacket(nil, fake.NewAddress(3), fake.NewAddress(0), fake.NewAddress(2), fake.NewAddress(4), fake.NewAddress(5), fake.NewAddress(999)),
			},
		},
		{
			// If there is no parent, and the destination is unknown, it sends
			// to nil.
			from:        fake.NewAddress(2),
			destination: []mino.Address{fake.NewAddress(999)},
			expected: map[mino.Address]router.Packet{
				nil: makePacket(nil, fake.NewAddress(2), fake.NewAddress(999)),
			},
		},
	}

	r = NewRouter(newFakeMemship(7), 2, fake.AddressFactory{})
	r.newTree(0, 2)

	for _, entry := range table {
		packet := makePacket(nil, entry.from, entry.destination...)
		data, err := packet.Serialize(context)
		require.NoError(t, err)
		result, err := r.Forward(entry.from, data, context)

		require.NoError(t, err)
		// require.True(t, relay.Equal(entry[2]), "expected %s to be %s",
		// relay, entry[2])
		compareMap(t, entry.expected, result)
	}
}

func TestRouter_getDest(t *testing.T) {
	r := Router{
		routingNodes: map[mino.Address]*treeNode{
			fake.NewAddress(0): {
				Addr:      fake.NewAddress(0),
				Index:     0,
				LastIndex: 0,
			},
			fake.NewAddress(1): {
				Index:     10,
				LastIndex: 0,
			},
		},
	}

	zerolog.SetGlobalLevel(zerolog.Disabled)
	defer zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	r.getDest(fake.NewAddress(0), fake.NewAddress(1))
}

// -----------------------------------------------------------------------------
// Utility functions

func makePacket(msg []byte, me mino.Address, dest ...mino.Address) router.Packet {
	return types.Packet{
		Source:  me,
		Dest:    dest,
		Message: msg,
		Seed:    0,
	}
}

func compareMap(t *testing.T, expected, result map[mino.Address]router.Packet) {
	require.Len(t, result, len(expected))

	for k, v := range expected {
		r, found := result[k]
		require.True(t, found, "entry with key", k, "not found in result")
		require.Equal(t, v.GetSource(), r.GetSource())
		require.Equal(t, v.GetDestination(), r.GetDestination())
	}
}

func newFakeMemship(n int) MembershipService {
	auth := fake.NewAuthority(n, fake.NewSigner)
	addrs := make([]mino.Address, 0, n)

	iter := auth.AddressIterator()
	for iter.HasNext() {
		addrs = append(addrs, iter.GetNext())
	}

	return fakeMemship{
		addrs: addrs,
	}
}

type fakeMemship struct {
	addrs []mino.Address
}

func (m fakeMemship) Get(id []byte) []mino.Address {
	return m.addrs
}

type dummyFac struct{}

func (f dummyFac) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	return fake.Message{}, nil
}
