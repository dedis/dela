package types

import (
	"encoding/binary"

	"go.dedis.ch/dela/mino/router"
	"go.dedis.ch/dela/serde"
)

type Handshake struct {
	depth uint16
}

func NewHandshake(depth uint16) Handshake {
	return Handshake{
		depth: depth,
	}
}

func (h Handshake) GetDepth() uint16 {
	return h.depth
}

func (h Handshake) Serialize(serde.Context) ([]byte, error) {
	buffer := make([]byte, 2)
	binary.LittleEndian.PutUint16(buffer, h.depth)

	return buffer, nil
}

type HandshakeFactory struct{}

func NewHandshakeFactory() HandshakeFactory {
	return HandshakeFactory{}
}

func (fac HandshakeFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	return fac.HandshakeOf(ctx, data)
}

func (HandshakeFactory) HandshakeOf(ctx serde.Context, data []byte) (router.Handshake, error) {
	depth := binary.LittleEndian.Uint16(data)

	return NewHandshake(depth), nil
}
