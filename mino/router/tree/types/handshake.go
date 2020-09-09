package types

import (
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
)

var hsFormats = registry.NewSimpleRegistry()

func RegisterHandshakeFormat(f serde.Format, e serde.FormatEngine) {
	hsFormats.Register(f, e)
}

type Handshake struct {
	height   int
	expected []mino.Address
}

func NewHandshake(height int, expected []mino.Address) Handshake {
	return Handshake{
		height:   height,
		expected: expected,
	}
}

func (h Handshake) GetHeight() int {
	return h.height
}

func (h Handshake) GetAddresses() []mino.Address {
	return h.expected
}

func (h Handshake) Serialize(ctx serde.Context) ([]byte, error) {
	format := hsFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, h)
	if err != nil {
		return nil, err
	}

	return data, nil
}

type HandshakeFactory struct {
	addrFac mino.AddressFactory
}

func NewHandshakeFactory(addrFac mino.AddressFactory) HandshakeFactory {
	return HandshakeFactory{
		addrFac: addrFac,
	}
}

func (fac HandshakeFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	return fac.HandshakeOf(ctx, data)
}

func (fac HandshakeFactory) HandshakeOf(ctx serde.Context, data []byte) (router.Handshake, error) {
	format := hsFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, AddrKey{}, fac.addrFac)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, err
	}

	hs, ok := msg.(Handshake)
	if !ok {
		return nil, xerrors.New("invalid handshake")
	}

	return hs, nil
}
