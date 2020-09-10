package types

import "go.dedis.ch/dela/serde"

// RegisterHandshakeFormat registers the engine for the provided format.
func RegisterHandshakeFormat(f serde.Format, e serde.FormatEngine) {
	hsFormats.Register(f, e)
}

// RegisterPacketFormat registers the engine for the provided format.
func RegisterPacketFormat(c serde.Format, f serde.FormatEngine) {
	packetFormat.Register(c, f)
}
