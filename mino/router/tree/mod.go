// Package tree is an implementation of the router.Router interface. It uses a
// static tree generation to route the packets.
package tree

import (
	"bytes"
	"math/rand"
	"regexp"
	"sort"

	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

var eachLine = regexp.MustCompile(`(?m)^(.+)$`)

// MembershipService is a service that is required by the router to get
// information about the participants so that it can generate the correct tree.
type MembershipService interface {
	Get(id []byte) []mino.Address
}

// Router is the routing service
//
// - implements router.Router
type Router struct {
	memship      MembershipService
	root         *treeNode
	routingNodes map[mino.Address]*treeNode
	initialized  bool
	height       int
}

// NewRouter returns a new router
func NewRouter(memship MembershipService, height int) *Router {
	r := &Router{
		memship: memship,
		height:  height,
	}

	return r
}

// MakePacket implements router.Router
func (r Router) MakePacket(me, to mino.Address, msg []byte) router.Packet {
	return Packet{
		source:  me,
		dest:    to,
		message: msg,
	}
}

// Forward implements router.Router. It returns the node that is able to relay
// the message (or correspond to the address). We are able to easily know the
// route because each address has an index corresponding to its node index on
// the tree that would comme from a depth-first pre-order enumeration of the
// nodes. For example:
//         root
//      /    |    \
//     0     3     6
//   / |   / |   / | \
//  1  2  4  5  7  8  9
// Then, each node keeps the range of index that it holds in its sub-tree with
// its "index" and "lastIndex" attributes. For example, node 3 will have index =
// 3 and lastIndex = 5. Now, if the root wants to know which of its children to
// contact in order to reach node 4, it then checks the "index" and "indexLast"
// for all its children, and see that for its child 3, 3 >= 4 <= 5, so the root
// will send its message to node 3.
//
func (r *Router) Forward(packet router.Packet) (mino.Address, error) {

	// TODO: decide when to (re-)compute the tree
	if !r.initialized {
		// TODO: get the seed from somewhere
		r.newTree(0, r.height)
		r.initialized = true
	}

	// If the sender is unknown, it returns the root of the tree (for example,
	// the source might be the fake orchestrator address).
	sourceNode, ok := r.routingNodes[packet.GetSource()]
	if !ok {
		return r.root.Addr, nil
	}

	// If the source equals the destination, it sends to the destination.
	if sourceNode.Addr.Equal(packet.GetDestination()) {
		return packet.GetDestination(), nil
	}

	target := r.routingNodes[packet.GetDestination()]

	// If the target address is not in the tree, it sends to its parent.
	if target == nil && sourceNode.Parent != nil {
		return sourceNode.Parent.Addr, nil
	}

	// If there is no parent and the destination is unknown, it sends to the
	// destination.
	if target == nil {
		return packet.GetDestination(), nil
	}

	// Find the correct children
	for _, c := range sourceNode.Children {
		if target.Index >= c.Index && target.Index <= c.LastIndex {
			return c.Addr, nil
		}
	}

	if sourceNode.Parent == nil {
		return nil, xerrors.Errorf("didn't find any children to send to %s, "+
			"and there is no root for %s", target.Addr, sourceNode.Addr)
	}

	return sourceNode.Parent.Addr, nil
}

// newTree builds a new tree
func (r *Router) newTree(seed int64, height int) error {
	addrs := r.memship.Get(nil)
	addrsBuf := make([][]byte, len(addrs))

	for i, addr := range addrs {
		addrBuf, err := addr.MarshalText()
		if err != nil {
			return xerrors.Errorf("failed to marshal addr: %v", err)
		}

		addrsBuf[i] = addrBuf
	}

	sort.Stable(Addresses{buffers: addrsBuf, addrs: addrs})
	rand.Seed(seed)
	rand.Shuffle(len(addrs), func(i, j int) {
		addrs[i], addrs[j] = addrs[j], addrs[i]
	})

	tree := buildTree(addrs[0], addrs[1:], height, 0, nil)

	routingNodes := make(map[mino.Address]*treeNode)
	routingNodes[addrs[0]] = tree

	tree.ForEach(func(n *treeNode) {
		if !n.Addr.Equal(addrs[0]) {
			routingNodes[n.Addr] = n
		}
	})

	r.root = tree
	r.routingNodes = routingNodes

	return nil
}

// OnFailure implements router.Router
func (r Router) OnFailure(to mino.Address) error {
	return xerrors.Errorf("routing to %v failed", to)
}

// Packet describes a routing packet
//
// - implements router.Packet
type Packet struct {
	source  mino.Address
	dest    mino.Address
	message []byte
}

// GetSource implements router.Packet
func (p Packet) GetSource() mino.Address {
	return p.source
}

// GetDestination implements router.Packet
func (p Packet) GetDestination() mino.Address {
	return p.dest
}

// GetMessage implements router.Packet
func (p Packet) GetMessage(ctx serde.Context, f serde.Factory) (serde.Message, error) {
	msg, err := f.Deserialize(ctx, p.message)
	if err != nil {
		return nil, xerrors.Errorf("failed to deserialize message: %v", err)
	}

	return msg, nil
}

// Addresses is a sortable structure for both addresses and their binary
// representation.
type Addresses struct {
	buffers [][]byte
	addrs   []mino.Address
}

func (a Addresses) Len() int {
	return len(a.buffers)
}

func (a Addresses) Swap(i, j int) {
	a.buffers[i], a.buffers[j] = a.buffers[j], a.buffers[i]
	a.addrs[i], a.addrs[j] = a.addrs[j], a.addrs[i]
}

func (a Addresses) Less(i, j int) bool {
	return bytes.Compare(a.buffers[i], a.buffers[j]) < 0
}
