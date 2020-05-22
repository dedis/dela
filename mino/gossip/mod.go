package gossip

import (
	"github.com/golang/protobuf/proto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

// Rumor is the message that must be gossiped through the network. It is using
// the identifier as a unique way to differentiate all the rumors.
type Rumor interface {
	encoding.Packable

	// GetID returns the unique identifier of the rumor.
	GetID() []byte
}

// RumorFactory is a factory to instantiate a rumor from its protobuf message.
type RumorFactory interface {
	// FromProto returns the rumor associated with the protobuf message.
	FromProto(proto.Message) (Rumor, error)
}

// Actor is an actor that can send rumor to a gossip network.
type Actor interface {
	// SetPlayers should change the list of participants that the actor should
	// send rumors to. It is up to the implementation to send to only a subset.
	SetPlayers(mino.Players)

	// Add should add the rumor in the set of rumors that must be spread to the
	// participants.
	Add(rumor Rumor) error

	// Close should clean any resource used by the actor.
	Close() error
}

// Gossiper is an abstraction of a message passing protocol that uses internally
// a gossip protocol.
type Gossiper interface {
	GetRumorFactory() RumorFactory

	// Rumors should return a channel populated with the new rumors.
	Rumors() <-chan Rumor

	// Listen should start to listen for rumors and returns a gossip actor.
	Listen() (Actor, error)
}
