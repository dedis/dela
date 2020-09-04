// Package tree is an implementation of the router.Router interface. It uses a
// static tree generation to route the packets.
package tree

import (
	"bytes"
	"math/rand"
	"regexp"
	"sort"
	"sync"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/mino/router/tree/types"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

var eachLine = regexp.MustCompile(`(?m)^(.+)$`)

const seed = int64(1234)

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
	memship      MembershipService
	root         *treeNode
	routingNodes map[mino.Address]*treeNode
	initialized  bool
	height       int
	fac          serde.Factory
}

// NewRouter returns a new router
func NewRouter(memship MembershipService, height int, f mino.AddressFactory) *Router {
	fac := types.NewPacketFactory(f)

	r := &Router{
		memship: memship,
		height:  height,
		lock:    new(sync.Mutex),
		fac:     fac,
	}

	return r
}

// MakePacket implements router.Router. We don't take the source address when
// creating the router because we often provide the router as argument when we
// create mino, and at that time we don't know yet our address.
func (r Router) MakePacket(me mino.Address, to []mino.Address, msg []byte) router.Packet {
	return types.Packet{
		Source:  me,
		Dest:    to,
		Message: msg,
		Seed:    seed,
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
func (r *Router) Forward(me mino.Address, data []byte,
	ctx serde.Context) (map[mino.Address]router.Packet, error) {

	r.lock.Lock()
	defer r.lock.Unlock()

	msg, err := r.fac.Deserialize(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("failed to deserialize packet: %v", err)
	}

	packet, ok := msg.(types.Packet)
	if !ok {
		return nil, xerrors.Errorf("expected to have %T, got: %T", packet, msg)
	}

	// TODO: decide when to (re-)compute the tree
	if !r.initialized {
		r.initialized = true
		r.newTree(packet.Seed, r.height)
	}

	packets := map[mino.Address]*types.Packet{}

	for _, addr := range packet.GetDestination() {

		to := r.getDest(me, addr)

		p := packets[to]
		if p == nil {
			p = &types.Packet{
				Source:  packet.Source,
				Dest:    []mino.Address{},
				Message: packet.Message,
				Seed:    packet.Seed,
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

func (r Router) getDest(from, to mino.Address) mino.Address {

	sourceNode := r.routingNodes[from]
	target := r.routingNodes[to]

	// If we don't know the source and the target we can't do anything
	if sourceNode == nil && target == nil {
		return nil
	}

	// If the sender is unknown, it returns the root of the tree (for example,
	// the source might be the fake orchestrator address).
	if sourceNode == nil {
		return r.root.Addr
	}

	// If the source equals the destination, it sends to the destination.
	if sourceNode.Addr.Equal(to) {
		return to
	}

	// If the target address is not in the tree, it sends to its parent.
	if target == nil && sourceNode.Parent != nil {
		return sourceNode.Parent.Addr
	}

	// If there is no parent and the destination is unknown, it sends to the
	// destination.
	if target == nil {
		return nil
	}

	// Find the correct children
	for _, c := range sourceNode.Children {
		if target.Index >= c.Index && target.Index <= c.LastIndex {
			return c.Addr
		}
	}

	if sourceNode.Parent == nil {
		// This error only happens if the tree is malformed, which should never
		// happen if newTree is used and is very grave if it does.
		dela.Logger.Fatal().Msgf("didn't find any children to send to %s, "+
			"and there is no root for %s", target.Addr, sourceNode.Addr)
		return nil
	}

	return sourceNode.Parent.Addr
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
