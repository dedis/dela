package dkg

import (
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/kyber/v3"
)

// DKG defines the primitives to run a DKG protocol
type DKG interface {
	Encrypt(message []byte) (K, C kyber.Point, remainder []byte, err error)
	Decrypt(K, C kyber.Point) ([]byte, error)
}

// Factory defines the factory of a DKG
type Factory interface {
	New(players mino.Players, threshold uint32) (DKG, error)
}
