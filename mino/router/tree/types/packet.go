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
	Source  mino.Address
	Dest    []mino.Address
	Message []byte
	Depth   int
}

// GetSource implements router.Packet
func (p Packet) GetSource() mino.Address {
	return p.Source
}

// GetDestination implements router.Packet
func (p Packet) GetDestination() []mino.Address {
	return p.Dest
}

// GetMessage implements router.Packet
func (p Packet) GetMessage() []byte {
	return p.Message
}

// Slice implements router.Packet
func (p *Packet) Slice(addr mino.Address) router.Packet {
	removed := false

	// in reverse order to remove from the slice "in place"
	for i := len(p.Dest) - 1; i >= 0; i-- {
		if p.Dest[i].Equal(addr) {
			p.Dest = append(p.Dest[:i], p.Dest[i+1:]...)
			removed = true
		}
	}

	if !removed {
		return nil
	}

	return &Packet{
		Source:  p.Source,
		Dest:    []mino.Address{addr},
		Message: p.Message,
		Depth:   p.Depth,
	}
}

// Serialize implements serde.Message
func (p Packet) Serialize(ctx serde.Context) ([]byte, error) {
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

	packet, ok := msg.(Packet)
	if !ok {
		return nil, xerrors.Errorf("expected to find type '%T', but found '%T'", packet, msg)
	}

	return &packet, nil
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
