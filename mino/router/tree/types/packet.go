package types

import (
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
)

var packetFormat = registry.NewSimpleRegistry()

// RegisterMessageFormat register the engine for the provided format.
func RegisterMessageFormat(c serde.Format, f serde.FormatEngine) {
	packetFormat.Register(c, f)
}

// Packet describes a tree routing packet
//
// - implements router.Packet
type Packet struct {
	src  mino.Address
	dest []mino.Address
	msg  []byte
}

func NewPacket(src mino.Address, msg []byte, dest ...mino.Address) *Packet {
	return &Packet{
		src:  src,
		dest: dest,
		msg:  msg,
	}
}

// GetSource implements router.Packet
func (p *Packet) GetSource() mino.Address {
	return p.src
}

// GetDestination implements router.Packet
func (p *Packet) GetDestination() []mino.Address {
	return append([]mino.Address{}, p.dest...)
}

// GetMessage implements router.Packet
func (p *Packet) GetMessage() []byte {
	return p.msg
}

func (p *Packet) Add(to mino.Address) {
	p.dest = append(p.dest, to)
}

// Slice implements router.Packet
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

// Serialize implements serde.Message
func (p *Packet) Serialize(ctx serde.Context) ([]byte, error) {
	format := packetFormat.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, p)
	if err != nil {
		return nil, xerrors.Errorf("failed to serialize packet: %v", err)
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

// PacketOf implements router.PacketFactory
func (f PacketFactory) PacketOf(ctx serde.Context, data []byte) (router.Packet, error) {
	msg, err := f.Deserialize(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("failed to deserialize packet: %v", err)
	}

	packet, ok := msg.(*Packet)
	if !ok {
		return nil, xerrors.Errorf("expected to find type '%T', but found '%T'", packet, msg)
	}

	return packet, nil
}

// Deserialize implements serde.Factory
func (f PacketFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := packetFormat.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, AddrKey{}, f.addrFactory)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode message: %v", err)
	}

	return msg, nil
}
