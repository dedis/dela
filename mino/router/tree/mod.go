// Package tree is an implementation of the router.Router interface. It uses a
// static tree generation to route the packets.
package tree

import (
	"bytes"
	"regexp"
	"sort"
	"sync"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/mino/router/tree/types"
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
	lock         *sync.Mutex
	root         *treeNode
	routingNodes map[mino.Address]*treeNode
	height       int
	fac          router.PacketFactory

	childs  map[mino.Address]struct{}
	parents map[mino.Address]struct{}

	curHeight int
}

// NewRouter returns a new router
func NewRouter(height int, f mino.AddressFactory) *Router {
	fac := NewPacketFactory(f)

	r := &Router{
		height: height,
		lock:   new(sync.Mutex),
		fac:    fac,

		childs:  make(map[mino.Address]struct{}),
		parents: make(map[mino.Address]struct{}),

		curHeight: -1,
	}

	return r
}

// NewPacketFactory returns a new packet factory.
func NewPacketFactory(f mino.AddressFactory) router.PacketFactory {
	return types.NewPacketFactory(f)
}

// GetPacketFactory implements router.Router
func (r Router) GetPacketFactory() router.PacketFactory {
	return r.fac
}

// MakePacket implements router.Router. We don't take the source address when
// creating the router because we often provide the router as argument when we
// create mino, and at that time we don't know yet our address.
func (r Router) MakePacket(me mino.Address, to []mino.Address, msg []byte) router.Packet {
	return &types.Packet{
		Source:  me,
		Dest:    to,
		Message: msg,
		Depth:   0, // the root packet is at the first level
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
func (r *Router) Forward(memship router.Membership, packet router.Packet) (map[mino.Address]router.Packet, error) {

	r.lock.Lock()
	defer r.lock.Unlock()

	treePacket, ok := packet.(*types.Packet)
	if !ok {
		return nil, xerrors.Errorf("expected to have %T, got: %T", treePacket, packet)
	}

	// We re-compute the tree if the packet does not come from a child and there
	// is at least one new address in the destination. If the packet comes from
	// a child to an unknown destination then we will send it back to our
	// parent.
	_, isChild := r.childs[memship.GetLocal()]
	if !isChild {
		r.parents[memship.GetLocal()] = struct{}{}
	}

	if !isChild && r.foundNewChild(packet.GetDestination()) || r.curHeight != r.height-treePacket.Depth {
		// fmt.Println(memship.GetLocal(), "new child found:", packet.GetDestination())
		for _, addr := range packet.GetDestination() {
			_, isParent := r.parents[addr]
			if !isParent && !addr.Equal(memship.GetLocal()) {
				r.childs[addr] = struct{}{}
			}
		}

		r.curHeight = r.height - treePacket.Depth
		r.newTree(r.curHeight)
		// r.root.Display(os.Stdout)
		// fmt.Println("childs:", r.childs, "is child", isChild, "parents:", r.parents)
	}

	packets := map[mino.Address]*types.Packet{}

	for _, addr := range treePacket.GetDestination() {

		to := r.getDest(addr)

		// to = addr
		// fmt.Println("from", memship.GetLocal(), "to", addr, "=", to)

		p := packets[to]
		if p == nil {
			p = &types.Packet{
				Source:  treePacket.Source,
				Dest:    []mino.Address{},
				Message: treePacket.Message,
				Depth:   treePacket.Depth + 1,
			}
			if to == nil {
				p.Depth -= 2
			} else if to.Equal(memship.GetLocal()) {
				p.Depth--
			}
			packets[to] = p
		}

		p.Dest = append(p.Dest, addr)
	}

	out := make(map[mino.Address]router.Packet)

	for k, v := range packets {
		out[k] = v
	}

	return out, nil
}

func (r *Router) foundNewChild(addrs []mino.Address) bool {
	for _, addr := range addrs {
		_, found := r.childs[addr]
		_, isParent := r.parents[addr]
		if !found && !isParent {
			return true
		}
	}

	return false
}

func (r Router) getDest(to mino.Address) mino.Address {

	// If the destination is one of our parent, then we can directly send the
	// message to it.
	_, isParent := r.parents[to]
	if isParent {
		return to
	}

	// If we don't know the target we can't do anything.
	target := r.routingNodes[to]
	if target == nil {
		return nil
	}

	// Find the correct children
	for _, c := range r.root.Children {
		if target.Index >= c.Index && target.Index <= c.LastIndex {
			return c.Addr
		}
	}

	// This error only happens if the tree is malformed, which should never
	// happen if newTree is used and is very grave if it does.
	dela.Logger.Fatal().Msgf("didn't find any children to send to %s, "+
		"and there is no parent", target.Addr)

	return nil
}

// newTree builds a new tree
func (r *Router) newTree(height int) error {
	addrsBuf := make([][]byte, 0)
	naddrs := make([]mino.Address, 0)

	for addr := range r.childs {

		addrBuf, err := addr.MarshalText()
		if err != nil {
			return xerrors.Errorf("failed to marshal addr: %v", err)
		}

		addrsBuf = append(addrsBuf, addrBuf)
		naddrs = append(naddrs, addr)
	}

	// This is convenient to keep a consistent growing tree
	sort.Stable(Addresses{buffers: addrsBuf, addrs: naddrs})
	// rand.Seed(0)
	// rand.Shuffle(len(naddrs), func(i, j int) {
	// 	naddrs[i], naddrs[j] = naddrs[j], naddrs[i]
	// })

	tree := buildTree(nil, naddrs, height, 0, nil)

	routingNodes := make(map[mino.Address]*treeNode)

	tree.ForEach(func(n *treeNode) {
		routingNodes[n.Addr] = n
	})

	r.root = tree
	r.routingNodes = routingNodes

	return nil
}

// OnFailure implements router.Router
func (r Router) OnFailure(to mino.Address) error {
	return xerrors.Errorf("routing to %v failed", to)
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
