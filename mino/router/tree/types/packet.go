// This file contains the implementation of the packet message.
//
// Documentation Last Review: 06.10.2020
//

package types

import (
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
)

var packetFormat = registry.NewSimpleRegistry()

// Packet describes a tree routing packet
//
// - implements router.Packet
type Packet struct {
	src  mino.Address
	dest []mino.Address
	msg  []byte
}

// NewPacket creates a new packet.
func NewPacket(src mino.Address, msg []byte, dest ...mino.Address) *Packet {
	return &Packet{
		src:  src,
		dest: dest,
		msg:  msg,
	}
}

// GetSource implements router.Packet. It returns the source address of the
// packet.
func (p *Packet) GetSource() mino.Address {
	return p.src
}

// GetDestination implements router.Packet. It returns a list of addresses where
// the packet should be send to.
func (p *Packet) GetDestination() []mino.Address {
	return append([]mino.Address{}, p.dest...)
}

// GetMessage implements router.Packet. It returns the byte buffer of the
// message.
func (p *Packet) GetMessage() []byte {
	return append([]byte{}, p.msg...)
}

// Add appends the address to the destination list, only if it does not exist
// already.
func (p *Packet) Add(to mino.Address) {
	for _, addr := range p.dest {
		if addr.Equal(to) {
			return
		}
	}

	p.dest = append(p.dest, to)
}

// Slice implements router.Packet. It removes the address from the destination
// list and returns a packet with this single destination, if it exists.
// Otherwise the packet stays unchanged.
func (p *Packet) Slice(addr mino.Address) router.Packet {
	removed := false

	// in reverse order to remove from the slice "in place"
	for i := len(p.dest) - 1; i >= 0; i-- {
		if p.dest[i].Equal(addr) {
			p.dest = append(p.dest[:i], p.dest[i+1:]...)
			removed = true
		}
	}

	if !removed {
		return nil
	}

	return &Packet{
		src:  p.src,
		dest: []mino.Address{addr},
		msg:  p.msg,
	}
}

// Serialize implements serde.Message. It returns the serialized data of the
// packet.
func (p *Packet) Serialize(ctx serde.Context) ([]byte, error) {
	format := packetFormat.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, p)
	if err != nil {
		return nil, xerrors.Errorf("packet format: %v", err)
	}

	return data, nil
}

// AddrKey is the key for the address factory.
type AddrKey struct{}

// PacketFactory is a factory for the packet.
//
// - implements serde.Factory
type PacketFactory struct {
	addrFactory mino.AddressFactory
}

// NewPacketFactory returns a factory for the packet.
func NewPacketFactory(f mino.AddressFactory) PacketFactory {
	return PacketFactory{
		addrFactory: f,
	}
}

// PacketOf implements router.PacketFactory. It populates the packet associated
// with the data if appropriate, otherwise it returns an error.
func (f PacketFactory) PacketOf(ctx serde.Context, data []byte) (router.Packet, error) {
	format := packetFormat.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, AddrKey{}, f.addrFactory)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("packet format: %v", err)
	}

	packet, ok := msg.(*Packet)
	if !ok {
		return nil, xerrors.Errorf("invalid packet '%T'", msg)
	}

	return packet, nil
}

// Deserialize implements serde.Factory. It populates the packet associated
// with the data if appropriate, otherwise it returns an error.
func (f PacketFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	return f.PacketOf(ctx, data)
}
