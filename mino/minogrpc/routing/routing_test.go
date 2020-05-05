package routing

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/internal/testing/fake"
	"go.dedis.ch/fabric/mino"
)

func TestNewTreeRouting(t *testing.T) {

	h := 3

	root := fake.NewAddress(-1)

	addrs := []mino.Address{
		fake.NewAddress(0), fake.NewAddress(1), fake.NewAddress(2),
		fake.NewAddress(3), fake.NewAddress(4), fake.NewAddress(5),
		fake.NewAddress(6), fake.NewAddress(7),
	}

	factory := NewTreeRoutingFactory(h, root, fake.AddressFactory{})

	iterator := fake.NewAddressIterator(addrs)

	routing, err := factory.FromIterator(&iterator)
	require.NoError(t, err)

	treeRouting, ok := routing.(*TreeRouting)
	require.True(t, ok)

	treeRouting.Display(os.Stdout)

	// Here is the deterministic tree that should be built:
	//
	// TreeRouting, Root: Node[fake.Address[-1]-index[-1]-lastIndex[7]](
	// 	Node[fake.Address[2]-index[0]-lastIndex[3]](
	// 		Node[fake.Address[4]-index[1]-lastIndex[1]](
	// 		)
	// 		Node[fake.Address[1]-index[2]-lastIndex[3]](
	// 			Node[fake.Address[3]-index[3]-lastIndex[3]](
	// 			)
	// 		)
	// 	)
	// 	Node[fake.Address[7]-index[4]-lastIndex[7]](
	// 		Node[fake.Address[6]-index[5]-lastIndex[5]](
	// 		)
	// 		Node[fake.Address[5]-index[6]-lastIndex[7]](
	// 			Node[fake.Address[0]-index[7]-lastIndex[7]](
	// 			)
	// 		)
	// 	)
	// )

	res, err := treeRouting.GetRoute(root, addrs[2])
	require.NoError(t, err)
	require.Equal(t, addrs[2], res)
	res, err = treeRouting.GetRoute(root, addrs[4])
	require.NoError(t, err)
	require.Equal(t, addrs[2], res)
	res, err = treeRouting.GetRoute(root, addrs[1])
	require.NoError(t, err)
	require.Equal(t, addrs[2], res)
	res, err = treeRouting.GetRoute(root, addrs[3])
	require.NoError(t, err)
	require.Equal(t, addrs[2], res)

	res, err = treeRouting.GetRoute(root, addrs[7])
	require.NoError(t, err)
	require.Equal(t, addrs[7], res)
	res, err = treeRouting.GetRoute(root, addrs[6])
	require.NoError(t, err)
	require.Equal(t, addrs[7], res)
	res, err = treeRouting.GetRoute(root, addrs[5])
	require.NoError(t, err)
	require.Equal(t, addrs[7], res)
	res, err = treeRouting.GetRoute(root, addrs[0])
	require.NoError(t, err)
	require.Equal(t, addrs[7], res)

	res, err = treeRouting.GetRoute(addrs[7], addrs[7])
	require.NoError(t, err)
	require.Equal(t, addrs[7], res)
	res, err = treeRouting.GetRoute(addrs[7], addrs[6])
	require.NoError(t, err)
	require.Equal(t, addrs[6], res)
	res, err = treeRouting.GetRoute(addrs[7], addrs[5])
	require.NoError(t, err)
	require.Equal(t, addrs[5], res)
	res, err = treeRouting.GetRoute(addrs[7], addrs[0])
	require.NoError(t, err)
	require.Equal(t, addrs[5], res)
}

func TestBuildNode(t *testing.T) {
	addr := fake.NewAddress(-1)
	addrs := []mino.Address{
		fake.NewAddress(0),
		fake.NewAddress(1),
		fake.NewAddress(2),
		fake.NewAddress(3),
		fake.NewAddress(4),
		fake.NewAddress(5),
		fake.NewAddress(6),
		fake.NewAddress(7),
	}

	node := buildTree(addr, addrs, 3, -1)
	compareNode(t, node, -1, 7, "fake.Address[-1]", 3)
	compareNode(t, node.Children[0], 0, 1, "fake.Address[0]", 1)
	compareNode(t, node.Children[0].Children[0], 1, 1, "fake.Address[1]", 0)
	compareNode(t, node.Children[1], 2, 4, "fake.Address[2]", 2)
	compareNode(t, node.Children[1].Children[0], 3, 3, "fake.Address[3]", 0)
	compareNode(t, node.Children[1].Children[1], 4, 4, "fake.Address[4]", 0)
	compareNode(t, node.Children[2], 5, 7, "fake.Address[5]", 2)
	compareNode(t, node.Children[2].Children[0], 6, 6, "fake.Address[6]", 0)
	compareNode(t, node.Children[2].Children[1], 7, 7, "fake.Address[7]", 0)
}

// -----------------------------------------------------------------------------
// Utility functions

func compareNode(t *testing.T, node *treeNode, index, lastIndex int, addrStr string,
	numChilds int) {

	require.Equal(t, index, node.Index)
	require.Equal(t, addrStr, node.Addr.String())
	require.Equal(t, numChilds, len(node.Children))
}
