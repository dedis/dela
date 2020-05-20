package routing

import (
	"bytes"
	"sort"
	"testing"

	proto "github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
	internal "go.dedis.ch/fabric/internal/testing"
	"go.dedis.ch/fabric/internal/testing/fake"
	"go.dedis.ch/fabric/mino"
)

func TestMessages(t *testing.T) {
	messages := []proto.Message{
		&TreeRoutingProto{},
	}

	for _, m := range messages {
		internal.CoverProtoMessage(t, m)
	}
}

func TestTreeRoutingFactory_GetAddressFactory(t *testing.T) {
	factory := NewTreeRoutingFactory(3, fake.AddressFactory{})
	require.NotNil(t, factory.GetAddressFactory())
}

func TestTreeRoutingFactory_FromIterator(t *testing.T) {
	factory := NewTreeRoutingFactory(3, fake.AddressFactory{})

	authority := fake.NewAuthority(10, fake.NewSigner)

	routing, err := factory.FromIterator(fake.NewAddress(0), authority.AddressIterator())
	require.NoError(t, err)
	require.NotNil(t, routing)
	// Root is in the iterator so we expect the length of the iterator.
	require.Len(t, routing.(*TreeRouting).routingNodes, 10)

	iter := fake.NewAddressIterator([]mino.Address{fake.NewBadAddress()})
	_, err = factory.FromIterator(fake.NewAddress(1), iter)
	require.EqualError(t, err, "failed to marshal addr 'fake.Address[0]': fake error")

	factory.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	_, err = factory.FromIterator(fake.NewAddress(0), authority.AddressIterator())
	require.EqualError(t, err,
		"failed to build routing: failed to write hash: fake error")
}

func TestTreeRoutingFactory_FromAny(t *testing.T) {
	factory := NewTreeRoutingFactory(3, fake.AddressFactory{})

	iter := mino.NewAddresses(fake.NewAddress(1)).AddressIterator()
	rting, err := factory.FromIterator(fake.NewAddress(0), iter)
	require.NoError(t, err)
	rtingAny, err := encoding.NewProtoEncoder().PackAny(rting)
	require.NoError(t, err)

	res, err := factory.FromAny(rtingAny)
	require.NoError(t, err)
	require.Equal(t, rting, res)

	factory.encoder = fake.BadUnmarshalAnyEncoder{}
	_, err = factory.FromAny(rtingAny)
	require.EqualError(t, err,
		"failed to unmarshal routing message: fake error")

	factory.encoder = encoding.NewProtoEncoder()
	factory.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	_, err = factory.FromAny(rtingAny)
	require.EqualError(t, err,
		"failed to build routing: failed to write hash: fake error")
}

func TestTreeRouting_GetRoute(t *testing.T) {
	h := 3

	root := fake.NewAddress(-1)

	addrs := []mino.Address{
		fake.NewAddress(0), fake.NewAddress(1), fake.NewAddress(2),
		fake.NewAddress(3), fake.NewAddress(4), fake.NewAddress(5),
		fake.NewAddress(6), fake.NewAddress(7),
	}

	factory := NewTreeRoutingFactory(h, fake.AddressFactory{})

	iterator := fake.NewAddressIterator(addrs)

	routing, err := factory.FromIterator(root, iterator)
	require.NoError(t, err)

	treeRouting, ok := routing.(*TreeRouting)
	require.True(t, ok)

	// treeRouting.Display(os.Stdout)

	// Here is the deterministic tree that should be built:
	//
	// TreeRouting, Root: Node[fake.Address[-1]-index[0]-lastIndex[8]](
	// 	Node[fake.Address[2]-index[1]-lastIndex[4]](
	// 		Node[fake.Address[4]-index[2]-lastIndex[4]](
	// 			Node[fake.Address[1]-index[3]-lastIndex[3]](
	// 			)
	// 			Node[fake.Address[3]-index[4]-lastIndex[4]](
	// 			)
	// 		)
	// 	)
	// 	Node[fake.Address[7]-index[5]-lastIndex[8]](
	// 		Node[fake.Address[6]-index[6]-lastIndex[8]](
	// 			Node[fake.Address[5]-index[7]-lastIndex[7]](
	// 			)
	// 			Node[fake.Address[0]-index[8]-lastIndex[8]](
	// 			)
	// 		)
	// 	)
	// )

	res := treeRouting.GetRoute(root, addrs[2])
	require.Equal(t, addrs[2], res)
	res = treeRouting.GetRoute(root, addrs[4])
	require.Equal(t, addrs[2], res)
	res = treeRouting.GetRoute(root, addrs[1])
	require.Equal(t, addrs[2], res)
	res = treeRouting.GetRoute(root, addrs[3])
	require.Equal(t, addrs[2], res)

	res = treeRouting.GetRoute(root, addrs[7])
	require.Equal(t, addrs[7], res)
	res = treeRouting.GetRoute(root, addrs[6])
	require.Equal(t, addrs[7], res)
	res = treeRouting.GetRoute(root, addrs[5])
	require.Equal(t, addrs[7], res)
	res = treeRouting.GetRoute(root, addrs[0])
	require.Equal(t, addrs[7], res)

	res = treeRouting.GetRoute(addrs[7], addrs[7])
	require.Equal(t, addrs[7], res)
	res = treeRouting.GetRoute(addrs[7], addrs[6])
	require.Equal(t, addrs[6], res)
	res = treeRouting.GetRoute(addrs[7], addrs[5])
	require.Equal(t, addrs[6], res)
	res = treeRouting.GetRoute(addrs[7], addrs[0])
	require.Equal(t, addrs[6], res)

	require.Nil(t, treeRouting.GetRoute(fake.NewAddress(999), addrs[5]))
	require.Nil(t, treeRouting.GetRoute(addrs[0], fake.NewAddress(999)))
	require.Nil(t, treeRouting.GetRoute(addrs[0], addrs[5]))
}

func TestTreeRouting_GetRoot(t *testing.T) {
	treeRouting := &TreeRouting{
		Root: &treeNode{Addr: fake.NewAddress(123)},
	}

	require.Equal(t, fake.NewAddress(123), treeRouting.GetRoot())
}

func TestTreeRouting_GetParent(t *testing.T) {
	authority := fake.NewAuthority(10, fake.NewSigner)
	factory := NewTreeRoutingFactory(3, fake.AddressFactory{})

	treeRouting, err := factory.FromIterator(authority.GetAddress(0), authority.AddressIterator())
	require.NoError(t, err)

	// treeRouting.(*TreeRouting).Display(os.Stdout)

	// Here is the deterministic tree that should be built:
	//
	// TreeRouting, Root: Node[fake.Address[0]-index[0]-lastIndex[9]](
	// 	Node[fake.Address[4]-index[1]-lastIndex[4]](
	// 		Node[fake.Address[2]-index[2]-lastIndex[4]](
	// 			Node[fake.Address[6]-index[3]-lastIndex[3]](
	// 			)
	// 			Node[fake.Address[1]-index[4]-lastIndex[4]](
	// 			)
	// 		)
	// 	)
	// 	Node[fake.Address[7]-index[5]-lastIndex[9]](
	// 		Node[fake.Address[5]-index[6]-lastIndex[7]](
	// 			Node[fake.Address[3]-index[7]-lastIndex[7]](
	// 			)
	// 		)
	// 		Node[fake.Address[8]-index[8]-lastIndex[9]](
	// 			Node[fake.Address[9]-index[9]-lastIndex[9]](
	// 			)
	// 		)
	// 	)
	// )

	parent := treeRouting.GetParent(authority.GetAddress(9))
	require.Equal(t, authority.GetAddress(8), parent)

	parent = treeRouting.GetParent(authority.GetAddress(8))
	require.Equal(t, authority.GetAddress(7), parent)

	parent = treeRouting.GetParent(authority.GetAddress(7))
	require.Equal(t, authority.GetAddress(0), parent)

	require.Nil(t, treeRouting.GetParent(authority.GetAddress(0)))
	require.Nil(t, treeRouting.GetParent(fake.NewAddress(999)))
}

func TestTreeRouting_GetDirectLinks(t *testing.T) {
	authority := fake.NewAuthority(10, fake.NewSigner)
	factory := NewTreeRoutingFactory(3, fake.AddressFactory{})

	treeRouting, err := factory.FromIterator(authority.GetAddress(0), authority.AddressIterator())
	require.NoError(t, err)

	// treeRouting.(*TreeRouting).Display(os.Stdout)

	// Here is the deterministic tree that should be built:
	//
	// TreeRouting, Root: Node[fake.Address[0]-index[0]-lastIndex[9]](
	// 	Node[fake.Address[4]-index[1]-lastIndex[4]](
	// 		Node[fake.Address[2]-index[2]-lastIndex[4]](
	// 			Node[fake.Address[6]-index[3]-lastIndex[3]](
	// 			)
	// 			Node[fake.Address[1]-index[4]-lastIndex[4]](
	// 			)
	// 		)
	// 	)
	// 	Node[fake.Address[7]-index[5]-lastIndex[9]](
	// 		Node[fake.Address[5]-index[6]-lastIndex[7]](
	// 			Node[fake.Address[3]-index[7]-lastIndex[7]](
	// 			)
	// 		)
	// 		Node[fake.Address[8]-index[8]-lastIndex[9]](
	// 			Node[fake.Address[9]-index[9]-lastIndex[9]](
	// 			)
	// 		)
	// 	)
	// )

	children := treeRouting.GetDirectLinks(authority.GetAddress(7))
	require.Len(t, children, 2)
	require.Equal(t, authority.GetAddress(5), children[0])
	require.Equal(t, authority.GetAddress(8), children[1])

	require.Len(t, treeRouting.GetDirectLinks(authority.GetAddress(9)), 0)
	require.Len(t, treeRouting.GetDirectLinks(fake.NewAddress(999)), 0)
}

func TestTreeRouting_Pack(t *testing.T) {
	n := 10

	authority := fake.NewAuthority(n, fake.NewSigner)
	factory := NewTreeRoutingFactory(3, fake.AddressFactory{})

	treeRouting, err := factory.FromIterator(authority.GetAddress(0), authority.AddressIterator())
	require.NoError(t, err)

	pb, err := treeRouting.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.NotNil(t, pb)
	require.NotEmpty(t, pb.(*TreeRoutingProto).GetRoot())
	require.Len(t, pb.(*TreeRoutingProto).GetAddrs(), n-1)

	treeRouting = &TreeRouting{routingNodes: map[mino.Address]*treeNode{
		fake.NewBadAddress(): {Addr: fake.NewBadAddress()},
	}}
	_, err = treeRouting.Pack(encoding.NewProtoEncoder())
	require.EqualError(t, err, "failed to marshal address: fake error")
}

func TestTreeRouting_Display(t *testing.T) {
	authority := fake.NewAuthority(3, fake.NewSigner)
	factory := NewTreeRoutingFactory(2, fake.AddressFactory{})

	treeRouting, err := factory.FromIterator(authority.GetAddress(0), authority.AddressIterator())
	require.NoError(t, err)

	buffer := new(bytes.Buffer)
	treeRouting.(*TreeRouting).Display(buffer)

	expected := `TreeRouting, Root: Node[fake.Address[0]-index[0]-lastIndex[2]](
	Node[fake.Address[1]-index[1]-lastIndex[2]](
		Node[fake.Address[2]-index[2]-lastIndex[2]](
		)
	)
)
`
	require.Equal(t, expected, buffer.String())
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
	// node.Display(os.Stdout)

	// Node[fake.Address[-1]-index[-1]-lastIndex[7]](
	// 	Node[fake.Address[0]-index[0]-lastIndex[3]](
	// 		Node[fake.Address[1]-index[1]-lastIndex[3]](
	// 			Node[fake.Address[2]-index[2]-lastIndex[2]](
	// 			)
	// 			Node[fake.Address[3]-index[3]-lastIndex[3]](
	// 			)
	// 		)
	// 	)
	// 	Node[fake.Address[4]-index[4]-lastIndex[7]](
	// 		Node[fake.Address[5]-index[5]-lastIndex[7]](
	// 			Node[fake.Address[6]-index[6]-lastIndex[6]](
	// 			)
	// 			Node[fake.Address[7]-index[7]-lastIndex[7]](
	// 			)
	// 		)
	// 	)
	// )

	compareNode(t, node, -1, 7, "fake.Address[-1]", 2)
	compareNode(t, node.Children[0], 0, 3, "fake.Address[0]", 1)
	compareNode(t, node.Children[0].Children[0], 1, 3, "fake.Address[1]", 2)
	compareNode(t, node.Children[0].Children[0].Children[0], 2, 2, "fake.Address[2]", 0)
	compareNode(t, node.Children[0].Children[0].Children[1], 3, 3, "fake.Address[3]", 0)
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
		node := buildTree(addrs[0], addrs[1:], h, 0)
		// fmt.Println("h =", h)
		// node.Display(os.Stdout)
		expected := h
		if expected >= n {
			expected = n - 1
		}
		require.Equal(t, expected, getHeight(node))
	}

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
