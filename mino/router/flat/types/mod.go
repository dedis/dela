// Package types implements the packet and handshake messages for the flat
// routing algorithm.
//
// The messages have been implemented in this isolated package so that it does
// not create cycle imports when importing the serde formats.
//
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
