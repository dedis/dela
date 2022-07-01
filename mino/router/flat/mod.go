// Package flat implements a flat routing that assumes a N-N connectivity among
// nodes.
//
package flat

import (
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/mino/router/flat/types"
	"golang.org/x/xerrors"
)

// Router is an implementation of a router producing direct routes
//
// - implements router.Router
type Router struct {
	packetFac router.PacketFactory
	hsFac     router.HandshakeFactory
}

// NewRouter returns a new router.
func NewRouter(f mino.AddressFactory) Router {
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

// New implements router.Router.
func (r Router) New(players mino.Players, me mino.Address) (router.RoutingTable, error) {
	addrs := make([]mino.Address, 0, players.Len())
	iter := players.AddressIterator()
	for iter.HasNext() {
		addrs = append(addrs, iter.GetNext())
	}

	return NewTable(addrs), nil
}

// GenerateTableFrom implements router.Router. It creates the routing table
// associated with the handshake that can contain some parameter.
func (r Router) GenerateTableFrom(h router.Handshake) (router.RoutingTable, error) {
	flatH := h.(types.Handshake)
	return NewTable(flatH.GetKnown()), nil
}

// Table is an empty routing table. We don't need one since we are using direct
// routing.
//
// - implements router.RoutingTable
type Table struct {
	known []mino.Address
}

// NewTable creates a new routing table
func NewTable(addrs []mino.Address) Table {
	return Table{
		known: addrs,
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
	return types.NewHandshake(t.known...)
}

// Forward implements router.RoutingTable. It takes a packet and split it into
// the different routes it should be forwarded to.
func (t Table) Forward(packet router.Packet) (router.Routes, router.Voids) {
	routes := make(router.Routes)
	voids := make(router.Voids)

	for _, dest := range packet.GetDestination() {

		var gateway mino.Address

		// check that we know the destination address, otherwise the gateway is
		// nil.
		for _, addr := range t.known {
			if dest.Equal(addr) {
				gateway = dest
				break
			}
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
	return xerrors.New("flat routing has no alternatives")
}
