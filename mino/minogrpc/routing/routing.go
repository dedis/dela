package routing

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	math "math"
	"math/rand"
	"regexp"
	"sort"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=plugins=grpc:./ ./message.proto

var eachLine = regexp.MustCompile(`(?m)^(.+)$`)

// Factory defines the primitive to create a Routing
type Factory interface {
	FromIterator(mino.AddressIterator) (Routing, error)
	FromAny(*any.Any) (Routing, error)
}

// Routing defines the functions needed to route messages
type Routing interface {
	encoding.Packable
	// GetRoute should return the gateway address for a corresponding addresse.
	// In a tree communication it is typically the address of the child that
	// contains the "to" address in its sub-tree.
	GetRoute(from, to mino.Address) (gateway mino.Address, err error)
	// GetDirectLinks return the direct links of the elements. In a tree routing
	// this is typically the children of the node.
	GetDirectLinks(from mino.Address) ([]mino.Address, error)
}

// TreeRouting holds the routing tree of a network. It allows each node of the
// tree to know which child it should contact in order to relay a message that
// is in it sub-tree.
//
// - implements Routing
type TreeRouting struct {
	Root           *treeNode
	routingNodes   map[string]*treeNode
	orchestratorID string
}

// TreeRoutingFactory defines the factory for tree routing
type TreeRoutingFactory struct {
	height         int
	rootAddr       mino.Address
	addrFactory    mino.AddressFactory
	orchestratorID string
}

// NewTreeRoutingFactory returns a new treeRoutingFactory.
func NewTreeRoutingFactory(height int, rootAddr mino.Address,
	addrFactory mino.AddressFactory, orchestratorID string) *TreeRoutingFactory {

	return &TreeRoutingFactory{
		height:         height,
		rootAddr:       rootAddr,
		addrFactory:    addrFactory,
		orchestratorID: orchestratorID,
	}
}

// FromIterator creates the network tree in a deterministic manner based on
// the addresses. The root address is automatically exluded if present.
func (t TreeRoutingFactory) FromIterator(iterator mino.AddressIterator) (Routing, error) {

	addrsBuf := make(addrsBuf, 0)
	for iterator.HasNext() {
		addr := iterator.GetNext()

		if addr.Equal(t.rootAddr) {
			continue
		}

		addrBuf, err := addr.MarshalText()
		if err != nil {
			return nil, xerrors.Errorf("failed to marshal addr '%s': %v", addr, err)
		}

		addrsBuf = append(addrsBuf, addrBuf)
	}

	routing, err := t.fromAddrBuf(addrsBuf, t.orchestratorID)
	if err != nil {
		return nil, xerrors.Errorf("failed to build from addrsBuf: %v", err)
	}

	return routing, nil
}

// FromAny creates the network tree in a deterministic manner based on the proto
// message encoded as any. It must not contain the root address, which is the
// case if the Pack() method has been used.
func (t TreeRoutingFactory) FromAny(m *any.Any) (Routing, error) {

	msg := &TreeRoutingProto{}
	err := ptypes.UnmarshalAny(m, msg)
	if err != nil {
		return nil, xerrors.Errorf("failed to unmarshal routing message: %v", err)
	}

	routing, err := t.fromAddrBuf(msg.Addrs, t.orchestratorID)
	if err != nil {
		return nil, xerrors.Errorf("failed to build from addrsBuf: %v", err)
	}

	return routing, nil
}

func (t TreeRoutingFactory) fromAddrBuf(addrsBuf addrsBuf,
	orchestratorID string) (Routing, error) {

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
	rand.Shuffle(len(addrsBuf), func(i, j int) {
		addrsBuf[i], addrsBuf[j] = addrsBuf[j], addrsBuf[i]
	})

	addrs := make([]mino.Address, len(addrsBuf))
	for i, addrBuf := range addrsBuf {
		addrs[i] = t.addrFactory.FromText(addrBuf)
	}

	// maximum number of direct connections each node can have. It is comupted
	// from the treeHeight and the total number of nodes. There are the
	// following relations:
	//
	// N: total number of nodes
	// D: number of direct connections wanted for each node
	// H: height of the network tree
	//
	// N = D^(H+1) - 1
	// D = sqrt[H+1](N+1)
	// H = log_D(N+1) - 1
	// N := float64(len(addrs) + 1)
	// d := int(math.Ceil(math.Pow(N+2.0, 1.0/float64(t.height+1))))

	tree := buildTree(t.rootAddr, addrs, t.height, 0)

	routingNodes := make(map[string]*treeNode)
	routingNodes[t.rootAddr.String()] = tree
	tree.ForEach(func(n *treeNode) {
		routingNodes[n.Addr.String()] = n
	})

	return &TreeRouting{
		routingNodes:   routingNodes,
		Root:           tree,
		orchestratorID: t.orchestratorID,
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
func (t TreeRouting) GetRoute(from, to mino.Address) (mino.Address, error) {

	// This is the case the main orchestrator want to send a message. The main
	// orchestrator is not a node by itself in the tree, but it has a connection
	// to the root. This is why each time the main orchestrator wants to send a
	// message we know we must relay it to the root. The main orchestrator
	// represents the entry point of the RPC stream that the client created.
	if from.String() == t.orchestratorID {
		return t.Root.Addr, nil
	}

	fromNode, ok := t.routingNodes[from.String()]
	if !ok {
		return nil, xerrors.Errorf("node with address '%s' not found",
			from.String())
	}

	if fromNode.Addr != nil && fromNode.Addr.Equal(to) {
		return to, nil
	}

	target, ok := t.routingNodes[to.String()]
	if !ok || target == nil {
		return nil, xerrors.Errorf("failed to find node '%s' in routingNode map",
			to.String())
	}

	for _, c := range fromNode.Children {
		if target.Index >= c.Index && target.Index <= c.LastIndex {
			return c.Addr, nil
		}
	}

	return nil, xerrors.Errorf("didn't find any route")
}

// GetDirectLinks returns the children
//
// - implements Routing
func (t TreeRouting) GetDirectLinks(from mino.Address) ([]mino.Address, error) {
	fromNode, ok := t.routingNodes[from.String()]
	if !ok {
		return nil, xerrors.Errorf("node with address '%s' not found",
			from.String())
	}

	res := make([]mino.Address, len(fromNode.Children))
	for i, c := range fromNode.Children {
		res[i] = c.Addr
	}

	return res, nil
}

// Pack returns the tree routing proto, which is the list of addresses without
// the root.
//
// - implements Routing
func (t TreeRouting) Pack(encoder encoding.ProtoMarshaler) (proto.Message, error) {
	addrs := make([][]byte, 0, len(t.routingNodes)-1)

	for _, node := range t.routingNodes {
		if node == t.Root {
			// the root is specified in the factory so we don't keep it
			continue
		}

		addrBuf, err := node.Addr.MarshalText()
		if err != nil {
			return nil, xerrors.Errorf("failed to marshal address: %v", err)
		}

		addrs = append(addrs, addrBuf)
	}

	msg := &TreeRoutingProto{
		Addrs: addrs,
	}

	return msg, nil
}

// Display displays an extensive string representation of the tree
func (t TreeRouting) Display(out io.Writer) {
	fmt.Fprintf(out, "TreeRouting, Root: ")
	t.Root.Display(out)
}

// buildTree builds the newtwork tree based on the list of addresses. The first
// call should have an index of 0.
func buildTree(addr mino.Address, addrs []mino.Address, h, index int) *treeNode {

	// the height can not be higher than the total number of nodes, ie. if there
	// are 10 nodes, then the maximum height we can have is 9. Here len(addrs)
	// represents the number of nodes-1 because the root addr is not included.
	if h > len(addrs) {
		h = len(addrs)
	}

	node := &treeNode{
		Index:     index,
		Addr:      addr,
		LastIndex: index + len(addrs),
	}

	children := make([]*treeNode, 0)

	N := float64(len(addrs) + 1)
	d := int(math.Round(math.Pow(N+1.0, 1.0/float64(h+1))))

	// This is a check that with the computed number of neighbours there will be
	// enought nodes to reach the given height. For example, if there are 6
	// addresses in the list, H = 4, then D = 1.51, which will be rounded to 2.
	// However, we can see that if we split the list in two, there will be 3
	// addresses in each sub-list that will have to reach a height of H-1 = 3,
	// which is impossible with 3 addresses. So this is why we decrement D.
	if len(addrs)/d < h {
		d = d - 1
	}

	// If we must build a height of 1 there is no other solutions than having
	// all the addresses as children.
	if h == 1 {
		d = len(addrs)
	}

	// k is the total number of elements in a sub tree
	//
	// In the case we want 2 direct connection per node and we have 7 addresses,
	// we then split the list into two parts, and there will be 3.5 addresses in
	// each part, re-arranged into 3 || 4:
	// a1 a2 a3 | a4 a5 a6 a7
	// "a1" will be the root of the first part
	// and "a4" the second one. We use k to delimit each part with k*i.
	k := float64(float64(len(addrs))) / float64(d)

	if k == 0 {
		children = []*treeNode{}
	} else if k < 1 {
		// This is the last level
		for i := 0; i < len(addrs); i++ {
			child := buildTree(addrs[i], []mino.Address{}, h-1, index+i+1)
			children = append(children, child)
		}
	} else {
		for i := 0; i < d; i++ {
			firstI := int(k * float64(i))
			lastI := int(k*float64(i) + k)
			child := buildTree(addrs[firstI], addrs[firstI+1:lastI], h-1,
				1+index+firstI)
			children = append(children, child)
		}
	}

	node.Children = children

	return node
}

// treeNode represents the address of a network node and the direct connections
// this network node has, represented by its children. The Index and LastIndex
// denotes the range of addresses the node has in its sub-tree.
type treeNode struct {
	Index     int
	LastIndex int
	Addr      mino.Address
	Children  []*treeNode
}

func (n treeNode) Display(out io.Writer) {
	fmt.Fprintf(out, "Node[%s-index[%d]-lastIndex[%d]](\n",
		n.Addr.String(), n.Index, n.LastIndex)

	for _, c := range n.Children {
		var buf bytes.Buffer
		c.Display(&buf)
		fmt.Fprint(out, eachLine.ReplaceAllString(buf.String(), "\t$1"))
	}

	fmt.Fprint(out, ")\n")
}

// ForEach calls f on each node in a depth-first pre-order manner
func (n *treeNode) ForEach(f func(n *treeNode)) {
	f(n)
	for _, c := range n.Children {
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
