package dkg

import (
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/suites"
)

// Factory defines the DKG Starter's factory
type Factory interface {
	New(pubKeys []kyber.Point, privKey kyber.Scalar, m mino.Mino,
		suite suites.Suite) (Starter, error)
}

// Starter defines the primitive to start a DKG protocol
type Starter interface {
	Start(players mino.Players, threshold uint32) (DKG, error)
}

// DKG defines the primitives to use a DKG protocol
type DKG interface {
	Encrypt(message []byte) (K, C kyber.Point, remainder []byte, err error)
	Decrypt(K, C kyber.Point) ([]byte, error)
	Reshare() error
}
