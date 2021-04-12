package shuffle

import (
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/kyber/v3"
)

// SHUFFLE defines the primitive to start a DKG protocol
type SHUFFLE interface {
	// Listen starts the RPC. This function should be called on each node that
	// wishes to participate in a SHUFFLE.
	Listen() (Actor, error)
}

// Actor defines the primitives to use a DKG protocol
type Actor interface {

	// Shuffle must be called by ONE of the actor to shuffle the list of ElGamal pairs.
	// Each node represented by a player must first execute Listen().
	Shuffle(co crypto.CollectiveAuthority, suiteName string, Ks []kyber.Point, Cs []kyber.Point,
		pubKey kyber.Point) (KsShuffled []kyber.Point, CsShuffled []kyber.Point, proof []byte, err error)

	// Verify allows to verify a Shuffle
	Verify(suiteName string, Ks []kyber.Point, Cs []kyber.Point, pubKey kyber.Point, KsShuffled []kyber.Point,
		CsShuffled []kyber.Point, proof []byte) (err error)

}