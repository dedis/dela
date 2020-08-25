package tree

import (
	"bytes"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
)

func TestBuildNode(t *testing.T) {
	addr := fake.NewAddress(0)
	addrs := []mino.Address{
		fake.NewAddress(1),
		fake.NewAddress(2),
		fake.NewAddress(3),
		fake.NewAddress(4),
		fake.NewAddress(5),
		fake.NewAddress(6),
		fake.NewAddress(7),
	}

	node := buildTree(addr, addrs, 3, 0, nil)
	// node.Display(os.Stdout)

	// Node[fake.Address[0]-index[0]-lastIndex[7]](
	// 	Node[fake.Address[1]-index[1]-lastIndex[3]](
	// 		Node[fake.Address[2]-index[2]-lastIndex[3]](
	// 			Node[fake.Address[3]-index[3]-lastIndex[3]]()
	// 		)
	// 	)
	// 	Node[fake.Address[4]-index[4]-lastIndex[7]](
	// 		Node[fake.Address[5]-index[5]-lastIndex[7]](
	// 			Node[fake.Address[6]-index[6]-lastIndex[6]]()
	// 			Node[fake.Address[7]-index[7]-lastIndex[7]]()
	// 		)
	// 	)
	// )

	compareNode(t, node, 0, 7, "fake.Address[0]", 2)
	compareNode(t, node.Children[0], 1, 3, "fake.Address[1]", 1)
	compareNode(t, node.Children[0].Children[0], 2, 3, "fake.Address[2]", 1)
	compareNode(t, node.Children[0].Children[0].Children[0], 3, 3, "fake.Address[3]", 0)
	compareNode(t, node.Children[1], 4, 7, "fake.Address[4]", 1)
	compareNode(t, node.Children[1].Children[0], 5, 7, "fake.Address[5]", 2)
	compareNode(t, node.Children[1].Children[0].Children[0], 6, 6, "fake.Address[6]", 0)
	compareNode(t, node.Children[1].Children[0].Children[1], 7, 7, "fake.Address[7]", 0)
}

func TestTreeShape(t *testing.T) {
	n := 100

	addrs := make([]mino.Address, n)
	for i := 0; i < n; i++ {
		addrs[i] = fake.NewAddress(i)
	}

	for h := 1; h < n+10; h++ {
		node := buildTree(addrs[0], addrs[1:], h, 0, nil)
		// fmt.Println("h =", h)
		// node.Display(os.Stdout)
		expected := h
		if expected >= n {
			expected = n - 1
		}
		require.Equal(t, expected, getHeight(node))
	}

}

func TestAddresses_Sort(t *testing.T) {
	addrs := Addresses{
		buffers: [][]byte{[]byte("B"), []byte("A")},
		addrs:   []mino.Address{fake.NewAddress(1), fake.NewAddress(0)},
	}

	sort.Sort(addrs)
	require.Equal(t, fake.NewAddress(0), addrs.addrs[0])
}

func TestTreeNode_Display(t *testing.T) {
	r := NewRouter(newFakeMemship(5), 2)
	r.newTree(0, 2)

	buffer := new(bytes.Buffer)
	r.root.Display(buffer)

	expected := `Node[fake.Address[2]-index[0]-lastIndex[4]](
	Node[fake.Address[3]-index[1]-lastIndex[2]](
		Node[fake.Address[1]-index[2]-lastIndex[2]]()
	)
	Node[fake.Address[0]-index[3]-lastIndex[4]](
		Node[fake.Address[4]-index[4]-lastIndex[4]]()
	)
)
`
	require.Equal(t, expected, buffer.String())
}

// -----------------------------------------------------------------------------
// Utility functions

func compareNode(t *testing.T, node *treeNode, index, lastIndex int, addrStr string,
	numChilds int) {

	require.Equal(t, index, node.Index, "node index", addrStr)
	require.Equal(t, addrStr, node.Addr.String(), "addr str", addrStr)
	require.Equal(t, numChilds, len(node.Children), "numChilds", addrStr)
}

func getHeight(node *treeNode) int {
	heights := make(sort.IntSlice, len(node.Children))
	for i, child := range node.Children {
		heights[i] = 1 + getHeight(child)
	}

	if len(heights) == 0 {
		return 0
	}

	heights.Sort()
	return heights[len(heights)-1]
}
