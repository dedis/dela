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
	// GetSource returns the source address of the message.
	GetSource() mino.Address

	// GetDestination returns the destination address of the message.
	GetDestination() mino.Address

	// GetMessage returns the message to be transmitted to the application when
	// the source address is the current node. Message is only deserialized when
	// needed.
	GetMessage(ctx serde.Context, f serde.Factory) (serde.Message, error)
}

// Router is the interface of the routing service. It provides the primitives to
// route a packet among a set of participants.
type Router interface {
	// MakePacket should be first called by the caller to set the specific
	// required attribute on the packet if needed, for example a seed.
	MakePacket(me, to mino.Address, msg []byte) Packet

	// Forward takes the destination address and a packet, processes the packet
	// if required, then returns the address to send it. It can either be the
	// destination or a hop address.
	Forward(packet Packet) (mino.Address, error)

	// OnFailure is used to announce that a packet failed to be routed. It
	// allows the router to find a different route. Forward can be called
	// afterwards to find an alternative route.
	OnFailure(to mino.Address) error
}
