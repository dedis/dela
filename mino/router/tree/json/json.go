package json

import (
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/router/tree/types"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	types.RegisterPacketFormat(serde.FormatJSON, packetFormat{})
	types.RegisterHandshakeFormat(serde.FormatJSON, hsFormat{})
}

// PacketJSON describes a JSON formatted packet
type PacketJSON struct {
	Source  []byte
	Dest    [][]byte
	Message []byte
}

// HandshakeJSON is the JSON message for the handshake.
type HandshakeJSON struct {
	Height    int
	Addresses [][]byte
}

// packetFormat is the engine to encode and decode Packets in JSON format.
//
// - implements serde.FormatEngine
type packetFormat struct{}

// Encode implements serde.FormatEngine. It returns the serialized data for the
// packet if appropriate, otherwise it returns an error.
func (f packetFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	packet, ok := msg.(*types.Packet)
	if !ok {
		return nil, xerrors.Errorf("unsupported message '%T'", msg)
	}

	source, err := packet.GetSource().MarshalText()
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal source addr: %v", err)
	}

	dest := make([][]byte, len(packet.GetDestination()))

	for i, addr := range packet.GetDestination() {
		addBuf, err := addr.MarshalText()
		if err != nil {
			return nil, xerrors.Errorf("failed to marshal dest addr: %v", err)
		}

		dest[i] = addBuf
	}

	p := PacketJSON{
		Source:  source,
		Dest:    dest,
		Message: packet.GetMessage(),
	}

	data, err := ctx.Marshal(p)
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal packet: %v", err)
	}

	return data, nil
}

// Decode implements serde.FormatEngine. It populates the packet if appropriate,
// otherwise it returns an error.
func (f packetFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	p := PacketJSON{}

	err := ctx.Unmarshal(data, &p)
	if err != nil {
		return nil, xerrors.Errorf("failed to unmarshal packet: %v", err)
	}

	factory := ctx.GetFactory(types.AddrKey{})

	fac, ok := factory.(mino.AddressFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid address factory '%T'", factory)
	}

	source := fac.FromText(p.Source)
	dest := make([]mino.Address, len(p.Dest))

	for i, buf := range p.Dest {
		dest[i] = fac.FromText(buf)
	}

	packet := types.NewPacket(source, p.Message, dest...)

	return packet, nil
}

// HandshakeFormat is the format engine to encode and decode handshake messages.
//
// - implements serde.FormatEngine
type hsFormat struct{}

// Encode implements serde.FormatEngine. It returns the serialized data for the
// handshake if appropriate, otherwise it returns an error.
func (hsFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	hs, ok := msg.(types.Handshake)
	if !ok {
		return nil, xerrors.Errorf("unsupported message '%T'", msg)
	}

	addrs := make([][]byte, len(hs.GetAddresses()))
	for i, addr := range hs.GetAddresses() {
		raw, err := addr.MarshalText()
		if err != nil {
			return nil, xerrors.Errorf("failed to marshal address: %v", err)
		}

		addrs[i] = raw
	}

	m := HandshakeJSON{
		Height:    hs.GetHeight(),
		Addresses: addrs,
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal handshake: %v", err)
	}

	return data, nil
}

// Decode implements serde.FormatEngine. It populates the handshake if
// appropriate, otherwise it returns an error.
func (hsFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := HandshakeJSON{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("failed to unmarshal: %v", err)
	}

	fac := ctx.GetFactory(types.AddrKey{})

	factory, ok := fac.(mino.AddressFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid address factory '%T'", fac)
	}

	addrs := make([]mino.Address, len(m.Addresses))
	for i, raw := range m.Addresses {
		addrs[i] = factory.FromText(raw)
	}

	return types.NewHandshake(m.Height, addrs...), nil
}
