package minogrpc

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	math "math"
	"math/rand"
	"sort"
	"strings"

	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

// RoutingFactory defines the primitive to create a Routing
type RoutingFactory interface {
	FromAddrs(addrs []mino.Address, opts map[string]interface{}) (Routing, error)
}

// Routing defines the functions needed to route messages
type Routing interface {
	// GetRoute should return the gateway address for a corresponding addresse.
	// In a tree communication it is typically the address of the child that
	// contains the "to" address in its sub-tree.
	GetRoute(to mino.Address) (gateway mino.Address, err error)
	// GetDirectLinks return the direct links of the elements. In a tree routing
	// this is typically the childs of the node.
	GetDirectLinks() []mino.Address
}

// TreeRouting implements Routing. It holds the routing tree of a network. It
// allows each node of the tree to know which child it should contact in order
// to relay a message that is in it sub-tree.
type TreeRouting struct {
	root         *treeNode
	me           *treeNode
	routingNodes map[string]*treeNode
}

// TreeRoutingOpts is the implementation of treeTreeRoutingOpts
var TreeRoutingOpts = treeRoutingOpts{
	// Addr is the address of the node, the value should be of type mino.Address
	Addr: "addr",
	// TreeHeight is the maximum tree height
	TreeHeight: "treeHeight",
}

type treeRoutingOpts struct {
	Addr       string
	TreeHeight string
}

// GetAddr parses the Addr option
func (t treeRoutingOpts) GetAddr(opts map[string]interface{}) (mino.Address, error) {
	addrItf, found := opts[t.Addr]
	if !found {
		return nil, xerrors.Errorf("didn't find address option")
	}
	addr, ok := addrItf.(mino.Address)
	if !ok {
		return nil, xerrors.Errorf("provided adress option is not a "+
			"mino.Address: %v", addrItf)
	}
	return addr, nil
}

// GetTreeHeight parses the tee height option
func (t treeRoutingOpts) GetTreeHeight(opts map[string]interface{}) (int, error) {
	treeHeightItf, found := opts[t.TreeHeight]
	if !found {
		return -1, xerrors.Errorf("didn't find treeHeight option")
	}
	treeHeight, ok := treeHeightItf.(int)
	if !ok {
		return -1, xerrors.Errorf("provided treeHeight option is not an int: %T",
			treeHeightItf)
	}
	return treeHeight, nil
}

// TreeRoutingFactory is the default TreeTreeRoutingFactory
var TreeRoutingFactory treeRoutingFactory

// TreeRoutingFactory defines the factory for tree routing
type treeRoutingFactory struct{}

// FromAddrs creates the network tree in a deterministic manner based on
// the list of addresses. Please be careful that the provided list of addresses
// is shuffled, which could affect subsequent use of this list, especially
// outside the function's scope.
func (t treeRoutingFactory) FromAddrs(addrs []mino.Address,
	opts map[string]interface{}) (Routing, error) {

	addr, err := TreeRoutingOpts.GetAddr(opts)
	if err != nil {
		return nil, xerrors.Errorf("failed to get address from opts: %v", err)
	}

	treeHeight, err := TreeRoutingOpts.GetTreeHeight(opts)
	if err != nil {
		return nil, xerrors.Errorf("failed to get treeHeight from opts: %v", err)
	}

	addrsBuf := make(addrsBuf, len(addrs))
	for i, addr := range addrs {
		addrBuf, err := addr.MarshalText()
		if err != nil {
			return nil, xerrors.Errorf("failed to marshal addr '%s': %v", addr, err)
		}
		addrsBuf[i] = addrBuf
	}
	sort.Stable(&addrsBuf)

	// We will use the hash of the addresses to set the random seed.
	hash := sha256.New()
	for _, addr := range addrsBuf {
		_, err := hash.Write(addr)
		if err != nil {
			fabric.Logger.Fatal().Msgf("failed to write hash: %v", err)
		}
	}

	seed := binary.LittleEndian.Uint64(hash.Sum(nil))

	// We shuffle the list of addresses, which will then be used to create the
	// network tree.
	rand.Seed(int64(seed))
	rand.Shuffle(len(addrs), func(i, j int) {
		addrs[i], addrs[j] = addrs[j], addrs[i]
	})

	// maximum number of direct connections each node can have. It is comupted
	// from the treeHeight and the total number of nodes. There are the
	// following relations:
	//
	// N: total number of nodes
	// D: number of direct connections wanted for each node
	// H: height of the network tree
	//
	// N = D^H
	// D = sqrt[H](N)
	// H = log_D N
	d := int(math.Ceil(math.Pow(float64(len(addrs)), 1.0/float64(treeHeight))))

	tree := buildTree(address{orchestratorAddr}, addrs, d, -1)

	routingNodes := make(map[string]*treeNode)
	routingNodes[addr.String()] = tree
	tree.ForEach(func(n *treeNode) {
		routingNodes[n.Addr.String()] = n
	})

	me, found := routingNodes[addr.String()]
	if !found {
		return nil, xerrors.Errorf("failed to find myself in the routingNode map")
	}

	return &TreeRouting{
		routingNodes: routingNodes,
		root:         tree,
		me:           me,
	}, nil
}

// GetRoute returns the node that is able to relay the message (or correspond to
// the address). We are able to easily know the route because each address has
// an index corresponding to its node index on the tree that would comme from a
// depth-first pre-order enumeration of the nodes. For example:
//         root
//      /    |    \
//     0     3     6
//   / |   / |   / | \
//  1  2  4  5  7  8  9
// Then each node keeps the range of index that it holds in its sub-tree with
// its "index" and "lastIndex" attributes. For example, node 3 will have index =
// 3 and lastIndex = 5. Now if the root wants to know which of its children to
// contact in order to reach node 4, it then checks the "index" and "indexLast"
// for all its children, an see that for its child 3, 3 >= 4 <= 5, so the root
// will send its message to node 3.
//
// - implements Routing
func (t TreeRouting) GetRoute(to mino.Address) (mino.Address, error) {

	if t.me.Addr != nil && t.me.Addr.Equal(to) {
		return to, nil
	}

	target, ok := t.routingNodes[to.String()]
	if !ok || target == nil {
		return nil, xerrors.Errorf("failed to find node '%s' in routingNode map",
			to.String())
	}

	for _, c := range t.me.Childs {
		if target.Index >= c.Index && target.Index <= c.LastIndex {
			return c.Addr, nil
		}
	}

	return nil, xerrors.Errorf("didn't find any route")
}

// GetDirectLinks returns the childs
//
// // - implements Routing
func (t TreeRouting) GetDirectLinks() []mino.Address {
	res := make([]mino.Address, len(t.me.Childs))
	for i, c := range t.me.Childs {
		res[i] = c.Addr
	}
	return res
}

// buildTree builds the newtwork tree based on the list of addresses. The first
// call should have an index of -1.
func buildTree(addr mino.Address, addrs []mino.Address, d int, index int) *treeNode {
	node := &treeNode{
		Index:     index,
		Addr:      addr,
		LastIndex: index + len(addrs),
	}

	childs := make([]*treeNode, 0, d)

	// k is the total number of elements in a sub tree
	//
	// In the case we want 2 direct connection per node and we have 7 addresses,
	// we then split the list into two parts, and there will be 3.5 addresses in
	// each part, re-arranged into 3 || 4:
	// a1 a2 a3 | a4 a5 a6 a7
	// "a1" will be the root of the first part
	// and "a4" the second one. We use k to delimit each part with k*i.
	k := float64(len(addrs)) / float64(d)

	if k == 0 {
		childs = []*treeNode{}
	} else if k < 1 {
		// This is the last level
		for i := 0; i < len(addrs); i++ {
			child := buildTree(addrs[i], []mino.Address{}, d, index+i+1)
			childs = append(childs, child)
		}
	} else {
		for i := 0; i < d; i++ {
			firstI := int(k * float64(i))
			lastI := int(k*float64(i) + k)
			child := buildTree(addrs[firstI], addrs[firstI+1:lastI], d,
				1+index+firstI)
			childs = append(childs, child)
		}
	}

	node.Childs = childs

	return node
}

// treeNode represents the address of a network node and the direct connections
// this network node has, represented by its children. The Index and LastIndex
// denotes the range of addresses the node has in its sub-tree.
type treeNode struct {
	Index     int
	LastIndex int
	Addr      mino.Address
	Childs    []*treeNode
}

func (n treeNode) String() string {
	out := new(strings.Builder)
	for _, c := range n.Childs {
		fmt.Fprint(out, eachLine.ReplaceAllString(c.String(), "\t$1")+"\n")
	}

	return fmt.Sprintf("Node[%s-index[%d]-lastIndex[%d]](\n%s)",
		n.Addr.String(), n.Index, n.LastIndex, out.String())
}

// ForEach calls f on each node in a depth-first pre-order manner
func (n *treeNode) ForEach(f func(n *treeNode)) {
	f(n)
	for _, c := range n.Childs {
		c.ForEach(f)
	}
}

// addrsBuf represents a slice of marshalled addresses that can be sorted
type addrsBuf [][]byte

func (a addrsBuf) Len() int {
	return len(a)
}

func (a addrsBuf) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

func (a addrsBuf) Less(i, j int) bool {
	return bytes.Compare(a[i], a[j]) < 0
}
