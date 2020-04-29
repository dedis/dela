package minogrpc

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/internal/testing/fake"
	"go.dedis.ch/fabric/mino"
)

func TestNewTreeRouting(t *testing.T) {
	h := 3
	addrs := []mino.Address{
		address{"0"}, address{"1"}, address{"2"}, address{"3"}, address{"4"},
		address{"5"}, address{"6"}, address{"7"},
	}

	// Here is the deterministic tree that should be built:
	//
	// Node[orchestrator_addr-index[-1]-lastIndex[7]](
	// 	Node[3-index[0]-lastIndex[3]](
	// 		Node[2-index[1]-lastIndex[1]](
	// 		)
	// 		Node[6-index[2]-lastIndex[3]](
	// 			Node[0-index[3]-lastIndex[3]](
	// 			)
	// 		)
	// 	)
	// 	Node[7-index[4]-lastIndex[7]](
	// 		Node[1-index[5]-lastIndex[5]](
	// 		)
	// 		Node[4-index[6]-lastIndex[7]](
	// 			Node[5-index[7]-lastIndex[7]](
	// 			)
	// 		)
	// 	)
	// )
	routing, err := TreeRoutingFactory.FromAddrs(addrs, map[string]interface{}{
		TreeRoutingOpts.Addr:       address{orchestratorAddr},
		TreeRoutingOpts.TreeHeight: h,
	})

	treeRouting, ok := routing.(*TreeRouting)
	require.True(t, ok)

	// fmt.Println(treeRouting.root)
	require.NoError(t, err)
	require.Equal(t, address{orchestratorAddr}, treeRouting.me.Addr)

	res, err := treeRouting.GetRoute(address{"1"})
	require.NoError(t, err)
	require.Equal(t, address{"7"}, res)
	res, err = treeRouting.GetRoute(address{"4"})
	require.NoError(t, err)
	require.Equal(t, address{"7"}, res)
	res, err = treeRouting.GetRoute(address{"5"})
	require.NoError(t, err)
	require.Equal(t, address{"7"}, res)

	res, err = treeRouting.GetRoute(address{"2"})
	require.NoError(t, err)
	require.Equal(t, address{"3"}, res)
	res, err = treeRouting.GetRoute(address{"6"})
	require.NoError(t, err)
	require.Equal(t, address{"3"}, res)
	res, err = treeRouting.GetRoute(address{"0"})
	require.NoError(t, err)
	require.Equal(t, address{"3"}, res)

	// Now I'm simulating me as Node[3-index[0]]
	treeRouting.me = treeRouting.me.Childs[0]

	res, err = treeRouting.GetRoute(address{"3"})
	require.NoError(t, err)
	require.Equal(t, address{"3"}, res)
	res, err = treeRouting.GetRoute(address{"2"})
	require.NoError(t, err)
	require.Equal(t, address{"2"}, res)
	res, err = treeRouting.GetRoute(address{"6"})
	require.NoError(t, err)
	require.Equal(t, address{"6"}, res)
	res, err = treeRouting.GetRoute(address{"0"})
	require.NoError(t, err)
	require.Equal(t, address{"6"}, res)
	// The node shouldn't be able to get its siblings
	res, err = treeRouting.GetRoute(address{"7"})
	require.Error(t, err, res)
	res, err = treeRouting.GetRoute(address{"1"})
	require.Error(t, err, res)
	res, err = treeRouting.GetRoute(address{"4"})
	require.Error(t, err, res)
	res, err = treeRouting.GetRoute(address{"5"})
	require.Error(t, err, res)
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
	compareNode(t, node.Childs[0], 0, 1, "fake.Address[0]", 1)
	compareNode(t, node.Childs[0].Childs[0], 1, 1, "fake.Address[1]", 0)
	compareNode(t, node.Childs[1], 2, 4, "fake.Address[2]", 2)
	compareNode(t, node.Childs[1].Childs[0], 3, 3, "fake.Address[3]", 0)
	compareNode(t, node.Childs[1].Childs[1], 4, 4, "fake.Address[4]", 0)
	compareNode(t, node.Childs[2], 5, 7, "fake.Address[5]", 2)
	compareNode(t, node.Childs[2].Childs[0], 6, 6, "fake.Address[6]", 0)
	compareNode(t, node.Childs[2].Childs[1], 7, 7, "fake.Address[7]", 0)
}

// -----------------------------------------------------------------------------
// Utility functions

func compareNode(t *testing.T, node *treeNode, index, lastIndex int, addrStr string,
	numChilds int) {

	require.Equal(t, index, node.Index)
	require.Equal(t, addrStr, node.Addr.String())
	require.Equal(t, numChilds, len(node.Childs))
}
