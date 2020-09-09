// Package tree is an implementation of the router.Router interface. It uses a
// static tree generation to route the packets.
package tree

import (
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/mino/router/tree/types"
)

// Router is the routing service
//
// - implements router.Router
type Router struct {
	packetFac router.PacketFactory
	hsFac     router.HandshakeFactory
}

// NewRouter returns a new router
func NewRouter(height int, f mino.AddressFactory) Router {
	fac := types.NewPacketFactory(f)
	hsFac := types.NewHandshakeFactory(f)

	r := Router{
		packetFac: fac,
		hsFac:     hsFac,
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
// is booting the protocol.
func (r Router) New(players mino.Players) (router.RoutingTable, error) {
	addrs := make([]mino.Address, 0, players.Len())
	iter := players.AddressIterator()
	for iter.HasNext() {
		addrs = append(addrs, iter.GetNext())
	}

	return NewTable(3, addrs), nil
}

// TableOf implements router.Router. It creates the routing table associated
// with the handshake that can contain some parameter.
func (r Router) TableOf(h router.Handshake) (router.RoutingTable, error) {
	treeH := h.(types.Handshake)

	return NewTable(treeH.GetHeight(), treeH.GetAddresses()), nil
}

// Table is a routing table that is using a tree structure to communicate
// between the nodes.
type Table struct {
	tree DynamicTree
}

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

// Prelude implements router.RoutingTable. It creates a handshake message that
// should be sent to the distant before as the first message when opening a
// relay to it.
func (t Table) Prelude(to mino.Address) router.Handshake {
	return types.NewHandshake(t.tree.GetHeight()-1, t.tree.GetAddressSet(to))
}

// Forward implements router.RoutingTable. It takes a packet and split it into
// the different routes it should be forwarded to.
func (t Table) Forward(packet router.Packet) (router.Routes, error) {
	routes := make(router.Routes)

	for _, dest := range packet.GetDestination() {
		gateway := t.tree.GetRoute(dest)

		p, ok := routes[gateway]
		if !ok {
			p = types.NewPacket(packet.GetSource(), packet.GetMessage())
			routes[gateway] = p
		}

		p.(*types.Packet).Add(dest)
	}

	return routes, nil
}

// OnFailure implements router.Router. It can be call by a node to announce that
// a route has failed so that the tree can be adapted.
func (t Table) OnFailure(to mino.Address) {
	panic("nope")
}
