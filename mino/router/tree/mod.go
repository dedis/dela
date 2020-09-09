// Package tree is an implementation of the router.Router interface. It uses a
// static tree generation to route the packets.
package tree

import (
	"bytes"

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
	hsFac := types.NewHandshakeFactory()

	r := Router{
		packetFac: fac,
		hsFac:     hsFac,
	}

	return r
}

// GetPacketFactory implements router.Router
func (r Router) GetPacketFactory() router.PacketFactory {
	return r.packetFac
}

func (r Router) GetHandshakeFactory() router.HandshakeFactory {
	return r.hsFac
}

func (r Router) New(mino.Players) (router.RoutingTable, error) {
	return Table{depth: 0}, nil
}

func (r Router) TableOf(h router.Handshake) (router.RoutingTable, error) {
	treeH := h.(types.Handshake)

	return Table{depth: treeH.GetDepth()}, nil
}

type Table struct {
	depth uint16
}

func (t Table) Make(src mino.Address, to []mino.Address, msg []byte) router.Packet {
	return types.NewPacket(src, msg, to...)
}

func (t Table) Prelude(mino.Address) router.Handshake {
	return types.NewHandshake(1)
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
func (t Table) Forward(packet router.Packet) (router.Routes, error) {
	routes := make(router.Routes)

	if t.depth > 0 {
		routes[nil] = packet

		return routes, nil
	}

	for _, dest := range packet.GetDestination() {
		routes[dest] = types.NewPacket(packet.GetSource(), packet.GetMessage(), dest)
	}

	return routes, nil
}

// OnFailure implements router.Router
func (t Table) OnFailure(to mino.Address) {
	panic("nope")
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
