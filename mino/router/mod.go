// Package router defines the primitives to route a packet among a set of
// participants.
package router

import (
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
)

// Packet is the type of message processed by the router. It contains
// information that will allow the message to be routed.
type Packet interface {
	serde.Message

	// GetSource returns the source address of the message.
	GetSource() mino.Address

	// GetDestination returns the destination address of the message.
	GetDestination() []mino.Address

	// GetMessage returns the message to be transmitted to the application when
	// the source address is the current node. Message is only deserialized when
	// needed.
	GetMessage() []byte

	// Slice removes the address from the destination if found and return a new
	// packet with the addr as the destination. If not found return nil.
	Slice(addr mino.Address) Packet
}

// PacketFactory describes the primitives to deserialize a packet
type PacketFactory interface {
	serde.Factory

	PacketOf(serde.Context, []byte) (Packet, error)
}

// Handshake is the message sent to the relay as the very first message. It is a
// way to provide parameters to the relays.
type Handshake interface {
	serde.Message
}

// HandshakeFactory is the factory to serialize and deserialize handshakes.
type HandshakeFactory interface {
	serde.Factory

	HandshakeOf(serde.Context, []byte) (Handshake, error)
}

// Router is the interface of the routing service. It provides the primitives to
// route a packet among a set of participants. The orchestrator address (if any)
// is not handled by the router. For that matter, the Packet.Slice function can
// be used to handle special cases with that address.
type Router interface {
	GetPacketFactory() PacketFactory

	GetHandshakeFactory() HandshakeFactory

	New(mino.Players) (RoutingTable, error)

	// TableOf returns the routing table associated to the handshake. A node
	// should be able to route any incoming packet after receiving one.
	TableOf(Handshake) (RoutingTable, error)
}

// Routes is a set of relay addresses where to send the packet.
type Routes map[mino.Address]Packet

// Voids is a set of addresses that cannot be addressed by the routing table.
type Voids map[mino.Address]error

// RoutingTable is built by the router and provides information about the
// routing of the packets.
type RoutingTable interface {
	// Make creates and returns a packet with the given source, destination and
	// payload.
	Make(src mino.Address, to []mino.Address, msg []byte) Packet

	// Prelude is called before a relay is opened to generate the handshake that
	// will be sent.
	Prelude(mino.Address) Handshake

	// Forward takes the destination address, unmarshal the packet, and, based
	// on its content, return a map of packets, where each element of the map
	// represents the destination as the key, and the packet to send as the
	// value. The simplest forward would be, if the destination is A,B,C, to
	// create a map with 3 entries:
	//
	//  {A: packet{to: [A]}, B: packet{to: [B]}, C: packet{to: [C]}}.
	//
	// For a tree routing, the first message will send a packet with all the
	// recipients to the root address (A in the following), like:
	//
	//	{A: packet{to: [A, B, C]}}
	//
	// Caution: it can return an error for unreachable addresses while still
	// returning correct routes.
	//
	Forward(packet Packet) (Routes, Voids)

	// OnFailure is used to announce that a packet failed to be routed. It
	// allows the router to find a different route. Forward can be called
	// afterwards to find an alternative route.
	OnFailure(to mino.Address) error
}
