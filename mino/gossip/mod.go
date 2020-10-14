// Package gossip defines an abstraction to gossip messages to a defined set of
// participants.
//
// Documentation Last Review: 06.10.2020
//
package gossip

import (
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
)

// Rumor is the message that must be gossiped through the network. It is using
// the identifier as a unique way to differentiate all the rumors.
type Rumor interface {
	serde.Message

	// GetID returns the unique identifier of the rumor.
	GetID() []byte
}

// Actor is an actor that can send rumor to a gossip network.
type Actor interface {
	// SetPlayers changes the list of participants that the actor should send
	// rumors to. It is up to the implementation to send to only a subset.
	SetPlayers(mino.Players)

	// Add adds the rumor in the set of rumors that must be spread to the
	// participants.
	Add(rumor Rumor) error

	// Close cleans any resource used by the actor.
	Close() error
}

// Gossiper is an abstraction of a message passing protocol that uses internally
// a gossip protocol.
type Gossiper interface {
	// Rumors returns a channel populated with the new rumors.
	Rumors() <-chan Rumor

	// Listen starts to listen for rumors and returns a gossip actor.
	Listen() (Actor, error)
}
