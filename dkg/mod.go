package dkg

import (
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/kyber/v3"
)

// DKG defines the primitive to start a DKG protocol
type DKG interface {
	Listen(players mino.Players, pubKeys []kyber.Point, threshold int) (Actor, error)
}

// Actor defines the primitives to use a DKG protocol
type Actor interface {
	Encrypt(message []byte) (K, C kyber.Point, remainder []byte, err error)
	Decrypt(K, C kyber.Point) ([]byte, error)
	Reshare() error
}
