package gossip

import (
	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

// Rumor is the message that must be gossiped through the network. It is using
// the identifier as a unique way of differentiate all the rumors.
type Rumor interface {
	encoding.Packable

	GetID() []byte
}

// Decoder is a function that decodes a message into a rumor implementation.
type Decoder = func(proto.Message) (Rumor, error)

// Gossiper is an abstraction of a message passing protocol that uses internally
// a gossip protocol.
type Gossiper interface {
	Add(rumor Rumor) error
	Rumors() <-chan Rumor
	Start(players mino.Players) error
	Stop() error
}
