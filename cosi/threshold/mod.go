// Package threshold is a stream-based implementation of a collective signing so
// that the orchestrator contacts only a subset of the participants. The
// collective signature allows a given threshold to be valid, which means that
// not all the participants need to return their signature for the protocol to
// end.
//
// Documentation Last Review: 05.10.2020
//
package threshold

import (
	"sync/atomic"

	"github.com/rs/zerolog"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/cosi/threshold/types"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
)

var (
	// protocolName denotes the value of the protocol span tag associated with
	// the `sign` protocol.
	protocolName = "sign"
)

func defaultThreshold(n int) int {
	return n
}

// OneThreshold is a threshold function to allow one failure.
func OneThreshold(n int) int {
	if n <= 0 {
		return 0
	}

	return n - 1
}

// ByzantineThreshold returns the minimum number of honest nodes required given
// `n` total nodes in a Byzantine Fault Tolerant system.
func ByzantineThreshold(n int) int {
	if n <= 0 {
		return 0
	}

	f := (n - 1) / 3

	return n - f
}

// Threshold is an implementation of the cosi.CollectiveSigning interface that
// is using streams to parallelize the work.
type Threshold struct {
	logger zerolog.Logger
	mino   mino.Mino
	signer crypto.AggregateSigner
	// Stores the cosi.Threshold function. It will always contain a valid
	// function by construction.
	thresholdFn atomic.Value
}

// NewThreshold returns a new instance of a threshold collective signature.
func NewThreshold(m mino.Mino, signer crypto.AggregateSigner) *Threshold {
	c := &Threshold{
		logger: dela.Logger.With().Str("addr", m.GetAddress().String()).Logger(),
		mino:   m,
		signer: signer,
	}

	// Force the cosi.Threshold type to allow later updates of the same type.
	c.thresholdFn.Store(cosi.Threshold(defaultThreshold))

	return c
}

// GetSigner implements cosi.CollectiveSigning. It returns the signer of the
// instance.
func (c *Threshold) GetSigner() crypto.Signer {
	return c.signer
}

// GetPublicKeyFactory implements cosi.CollectiveSigning. It returns the public
// key factory.
func (c *Threshold) GetPublicKeyFactory() crypto.PublicKeyFactory {
	return c.signer.GetPublicKeyFactory()
}

// GetSignatureFactory implements cosi.CollectiveSigning. It returns the
// signature factory.
func (c *Threshold) GetSignatureFactory() crypto.SignatureFactory {
	return types.NewSignatureFactory(c.signer.GetSignatureFactory())
}

// GetVerifierFactory implements cosi.CollectiveSigning. It returns the verifier
// factory.
func (c *Threshold) GetVerifierFactory() crypto.VerifierFactory {
	return types.NewThresholdVerifierFactory(c.signer.GetVerifierFactory())
}

// SetThreshold implements cosi.CollectiveSigning. It sets a new threshold
// function.
func (c *Threshold) SetThreshold(fn cosi.Threshold) {
	if fn == nil {
		return
	}

	c.thresholdFn.Store(fn)
}

// Listen implements cosi.CollectiveSigning. It creates the rpc endpoint and
// returns the actor that can trigger a collective signature.
func (c *Threshold) Listen(r cosi.Reactor) (cosi.Actor, error) {
	factory := cosi.NewMessageFactory(r, c.signer.GetSignatureFactory())

	actor := thresholdActor{
		Threshold: c,
		me:        c.mino.GetAddress(),
		rpc:       mino.MustCreateRPC(c.mino, "cosi", newHandler(c, r), factory),
		reactor:   r,
	}

	return actor, nil
}
