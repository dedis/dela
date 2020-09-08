package json

import (
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/router/tree/types"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	types.RegisterMessageFormat(serde.FormatJSON, newpacketFormat())
}

// packetFormat is the engine to encode and decode Packets in JSON format.
//
// - implements serde.FormatEngine
type packetFormat struct{}

func newpacketFormat() packetFormat {
	return packetFormat{}
}

// Encode implements serde.FormatEngine
func (f packetFormat) Encode(ctx serde.Context, message serde.Message) ([]byte, error) {
	packet, ok := message.(types.Packet)
	if !ok {
		return nil, xerrors.Errorf("unexpected type: %T != %T", packet, message)
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

	p := Packet{
		Source:  source,
		Dest:    dest,
		Message: packet.Message,
		Depth:   packet.Depth,
	}

	data, err := ctx.Marshal(p)
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal packet: %v", err)
	}

	return data, nil
}

// Decode implements serde.FormatEngine
func (f packetFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	p := Packet{}

	err := ctx.Unmarshal(data, &p)
	if err != nil {
		return nil, xerrors.Errorf("failed to unmarshal packet: %v", err)
	}

	factory := ctx.GetFactory(types.AddrKey{})

	fac, ok := factory.(mino.AddressFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid factory of type '%T'", factory)
	}

	source := fac.FromText(p.Source)
	dest := make([]mino.Address, len(p.Dest))

	for i, buf := range p.Dest {
		dest[i] = fac.FromText(buf)
	}

	packet := types.Packet{
		Source:  source,
		Dest:    dest,
		Message: p.Message,
		Depth:   p.Depth,
	}

	return packet, nil
}

// Packet describes a JSON formatted packet
type Packet struct {
	Source  []byte
	Dest    [][]byte
	Message []byte
	Depth   int
}
