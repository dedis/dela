package tree

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc"
	"go.dedis.ch/dela/serde/json"
)

func TestForward(t *testing.T) {
	r := NewRouter(newFakeMemship(7), 2)
	r.newTree(0, 2)

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

	// FROM - TO - RELAY
	table := [][]mino.Address{
		// if the source is unknown, it sends to the root (this is the case
		// when we use an orchestrator address, that is not part of the tree)
		{fake.NewAddress(-1), fake.NewAddress(-99), fake.NewAddress(2)},

		{fake.NewAddress(2), fake.NewAddress(2), fake.NewAddress(2)},
		{fake.NewAddress(2), fake.NewAddress(4), fake.NewAddress(4)},
		{fake.NewAddress(2), fake.NewAddress(5), fake.NewAddress(4)},
		{fake.NewAddress(2), fake.NewAddress(0), fake.NewAddress(4)},
		{fake.NewAddress(2), fake.NewAddress(3), fake.NewAddress(3)},
		{fake.NewAddress(2), fake.NewAddress(1), fake.NewAddress(3)},
		{fake.NewAddress(2), fake.NewAddress(6), fake.NewAddress(3)},
		// when the destination address is unknown, it sends directly to the
		// destination
		{fake.NewAddress(2), fake.NewAddress(-1), fake.NewAddress(-1)},

		{fake.NewAddress(4), fake.NewAddress(2), fake.NewAddress(2)},
		{fake.NewAddress(4), fake.NewAddress(4), fake.NewAddress(4)},
		{fake.NewAddress(4), fake.NewAddress(5), fake.NewAddress(5)},
		{fake.NewAddress(4), fake.NewAddress(0), fake.NewAddress(0)},
		{fake.NewAddress(4), fake.NewAddress(3), fake.NewAddress(2)},
		{fake.NewAddress(4), fake.NewAddress(1), fake.NewAddress(2)},
		{fake.NewAddress(4), fake.NewAddress(6), fake.NewAddress(2)},
		{fake.NewAddress(4), fake.NewAddress(-1), fake.NewAddress(2)},

		{fake.NewAddress(5), fake.NewAddress(2), fake.NewAddress(4)},
		{fake.NewAddress(5), fake.NewAddress(4), fake.NewAddress(4)},
		{fake.NewAddress(5), fake.NewAddress(5), fake.NewAddress(5)},
		{fake.NewAddress(5), fake.NewAddress(0), fake.NewAddress(4)},
		{fake.NewAddress(5), fake.NewAddress(3), fake.NewAddress(4)},
		{fake.NewAddress(5), fake.NewAddress(1), fake.NewAddress(4)},
		{fake.NewAddress(5), fake.NewAddress(6), fake.NewAddress(4)},
		{fake.NewAddress(5), fake.NewAddress(-1), fake.NewAddress(4)},

		{fake.NewAddress(0), fake.NewAddress(2), fake.NewAddress(4)},
		{fake.NewAddress(0), fake.NewAddress(4), fake.NewAddress(4)},
		{fake.NewAddress(0), fake.NewAddress(5), fake.NewAddress(4)},
		{fake.NewAddress(0), fake.NewAddress(0), fake.NewAddress(0)},
		{fake.NewAddress(0), fake.NewAddress(3), fake.NewAddress(4)},
		{fake.NewAddress(0), fake.NewAddress(1), fake.NewAddress(4)},
		{fake.NewAddress(0), fake.NewAddress(6), fake.NewAddress(4)},
		{fake.NewAddress(0), fake.NewAddress(-1), fake.NewAddress(4)},

		{fake.NewAddress(3), fake.NewAddress(2), fake.NewAddress(2)},
		{fake.NewAddress(3), fake.NewAddress(4), fake.NewAddress(2)},
		{fake.NewAddress(3), fake.NewAddress(5), fake.NewAddress(2)},
		{fake.NewAddress(3), fake.NewAddress(0), fake.NewAddress(2)},
		{fake.NewAddress(3), fake.NewAddress(3), fake.NewAddress(3)},
		{fake.NewAddress(3), fake.NewAddress(1), fake.NewAddress(1)},
		{fake.NewAddress(3), fake.NewAddress(6), fake.NewAddress(6)},
		{fake.NewAddress(3), fake.NewAddress(-1), fake.NewAddress(2)},

		{fake.NewAddress(1), fake.NewAddress(2), fake.NewAddress(3)},
		{fake.NewAddress(1), fake.NewAddress(4), fake.NewAddress(3)},
		{fake.NewAddress(1), fake.NewAddress(5), fake.NewAddress(3)},
		{fake.NewAddress(1), fake.NewAddress(0), fake.NewAddress(3)},
		{fake.NewAddress(1), fake.NewAddress(3), fake.NewAddress(3)},
		{fake.NewAddress(1), fake.NewAddress(1), fake.NewAddress(1)},
		{fake.NewAddress(1), fake.NewAddress(6), fake.NewAddress(3)},
		{fake.NewAddress(1), fake.NewAddress(-1), fake.NewAddress(3)},

		{fake.NewAddress(6), fake.NewAddress(2), fake.NewAddress(3)},
		{fake.NewAddress(6), fake.NewAddress(4), fake.NewAddress(3)},
		{fake.NewAddress(6), fake.NewAddress(5), fake.NewAddress(3)},
		{fake.NewAddress(6), fake.NewAddress(0), fake.NewAddress(3)},
		{fake.NewAddress(6), fake.NewAddress(3), fake.NewAddress(3)},
		{fake.NewAddress(6), fake.NewAddress(1), fake.NewAddress(3)},
		{fake.NewAddress(6), fake.NewAddress(6), fake.NewAddress(6)},
		{fake.NewAddress(6), fake.NewAddress(-1), fake.NewAddress(3)},
	}

	context := json.NewContext()

	for _, entry := range table {
		packet := r.MakePacket(entry[0], []mino.Address{entry[1]}, nil)
		data, err := packet.Serialize(context)
		require.NoError(t, err)
		_, err = r.Forward(entry[0], data, context, minogrpc.AddressFactory{})

		require.NoError(t, err)
		// require.True(t, relay.Equal(entry[2]), "expected %s to be %s",
		// relay, entry[2])
	}
}

// -----------------------------------------------------------------------------
// Utility functions

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
