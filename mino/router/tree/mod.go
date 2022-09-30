// Package tree is an implementation of a tree-based routing algorithm.
//
// The router creates a routing table for each protocol that will progressively
// build a tree. The tree will adapt to unresponsive participants but it is not
// resilient to faults happening after the table has been generated.
//
// The resulting tree will be balanced to limit the number of hops you need to
// send a message, while also trying to limit the number of connections per
// node. The routes are built upon requests so that the interior nodes of the
// tree are the first participants to be contacted.
//
// Documentation Last Review: 06.10.2020
package tree

import (
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/mino/router/tree/types"
	"golang.org/x/xerrors"
)

const defaultHeight = 3

// Router is an implementation of a router producing routes with an algorithm
// based on tree.
//
// - implements router.Router
type Router struct {
	maxHeight int
	packetFac router.PacketFactory
	hsFac     router.HandshakeFactory
}

// RouterOption is the signature of the option constructor
type RouterOption func(*Router)

// WithHeight allows to specify the maximum height of the tree
// when calling NewRouter
func WithHeight(maxHeight int) RouterOption {
	return func(r *Router) {
		r.maxHeight = maxHeight
	}
}

// NewRouter returns a new router
func NewRouter(f mino.AddressFactory, options ...RouterOption) Router {
	fac := types.NewPacketFactory(f)
	hsFac := types.NewHandshakeFactory(f)

	r := Router{
		maxHeight: defaultHeight,
		packetFac: fac,
		hsFac:     hsFac,
	}

	// Loop through each option
	for _, opt := range options {
		opt(&r)
	}
	return r
}

// GetPacketFactory implements router.Router. It returns the packet factory.
func (r Router) GetPacketFactory() router.PacketFactory {
	return r.packetFac
}

// GetHandshakeFactory implements router.Router. It returns the handshake
// factory.
func (r Router) GetHandshakeFactory() router.HandshakeFactory {
	return r.hsFac
}

// New implements router.Router. It creates the routing table for the node that
// is booting the protocol. This node will be the root of the tree.
func (r Router) New(players mino.Players, _ mino.Address) (router.RoutingTable, error) {
	addrs := make([]mino.Address, 0, players.Len())
	iter := players.AddressIterator()
	for iter.HasNext() {
		addrs = append(addrs, iter.GetNext())
	}

	return NewTable(r.maxHeight, addrs), nil
}

// GenerateTableFrom implements router.Router. It creates the routing table
// associated with the handshake that can contain some parameter.
func (r Router) GenerateTableFrom(h router.Handshake) (router.RoutingTable, error) {
	treeH := h.(types.Handshake)

	return NewTable(treeH.GetHeight(), treeH.GetAddresses()), nil
}

// Table is a routing table that is using a tree structure to communicate
// between the nodes.
//
// - implements router.RoutingTable
type Table struct {
	tree Tree
}

// NewTable creates a new routing table for the given addresses.
func NewTable(height int, expected []mino.Address) Table {
	return Table{
		tree: NewTree(height, expected),
	}
}

// Make implements router.RoutingTable. It creates a packet with the source
// address, the destination addresses and the payload.
func (t Table) Make(src mino.Address, to []mino.Address, msg []byte) router.Packet {
	return types.NewPacket(src, msg, to...)
}

// PrepareHandshakeFor implements router.RoutingTable. It creates a handshake
// message that should be sent to the distant peer when opening a relay to it.
// The peer will then generate its own routing table based on the handshake.
func (t Table) PrepareHandshakeFor(to mino.Address) router.Handshake {
	newHeight := t.tree.GetMaxHeight() - 1

	return types.NewHandshake(newHeight, t.tree.GetChildren(to)...)
}

// Forward implements router.RoutingTable. It takes a packet and split it into
// the different routes it should be forwarded to.
func (t Table) Forward(packet router.Packet) (router.Routes, router.Voids) {
	routes := make(router.Routes)
	voids := make(router.Voids)

	for _, dest := range packet.GetDestination() {
		gateway, err := t.tree.GetRoute(dest)
		if err != nil {
			voids[dest] = router.Void{Error: err}
			continue
		}

		p, ok := routes[gateway]
		if !ok {
			p = types.NewPacket(packet.GetSource(), packet.GetMessage())
			routes[gateway] = p
		}

		p.(*types.Packet).Add(dest)
	}

	return routes, voids
}

// OnFailure implements router.Router. The tree will try to adapt itself to
// reach the address, but it will return an error if the address is a direct
// branch of the tree.
func (t Table) OnFailure(to mino.Address) error {
	if t.tree.GetMaxHeight() <= 1 {
		// When the node does only have leafs, it will simply return an error to
		// announce the address as unreachable.
		return xerrors.New("address is unreachable")
	}

	t.tree.Remove(to)

	return nil
}
