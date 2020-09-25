package threshold

import (
	"sync/atomic"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/cosi/threshold/types"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
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

// ByzantineThreshold is a threshold function to allow a threshold number of
// failures and comply to the Byzantine Fault Tolerance theorem.
func ByzantineThreshold(n int) int {
	if n <= 0 {
		return 0
	}

	f := (n - 1) / 3

	return n - f
}

// CoSi is an implementation of the cosi.CollectiveSigning interface that is
// using streams to parallelize the work.
type CoSi struct {
	mino      mino.Mino
	signer    crypto.AggregateSigner
	threshold atomic.Value
}

// NewCoSi returns a new instance.
func NewCoSi(m mino.Mino, signer crypto.AggregateSigner) *CoSi {
	c := &CoSi{
		mino:   m,
		signer: signer,
	}

	// Force the cosi.Threshold type to allow later updates of the same type.
	c.threshold.Store(cosi.Threshold(defaultThreshold))

	return c
}

// GetSigner implements cosi.CollectiveSigning. It returns the signer of the
// instance.
func (c *CoSi) GetSigner() crypto.Signer {
	return c.signer
}

// GetPublicKeyFactory implements cosi.CollectiveSigning. It returns the public
// key factory.
func (c *CoSi) GetPublicKeyFactory() crypto.PublicKeyFactory {
	return c.signer.GetPublicKeyFactory()
}

// GetSignatureFactory implements cosi.CollectiveSigning. It returns the
// signature factory.
func (c *CoSi) GetSignatureFactory() crypto.SignatureFactory {
	return types.NewSignatureFactory(c.signer.GetSignatureFactory())
}

// GetVerifierFactory implements cosi.CollectiveSigning. It returns the verifier
// factory.
func (c *CoSi) GetVerifierFactory() crypto.VerifierFactory {
	return types.NewThresholdVerifierFactory(c.signer.GetVerifierFactory())
}

// SetThreshold implements cosi.CollectiveSigning. It sets a new threshold
// function.
func (c *CoSi) SetThreshold(fn cosi.Threshold) {
	if fn == nil {
		return
	}

	c.threshold.Store(fn)
}

// Listen implements cosi.CollectiveSigning.
func (c *CoSi) Listen(r cosi.Reactor) (cosi.Actor, error) {
	factory := cosi.NewMessageFactory(r, c.signer.GetSignatureFactory())

	rpc, err := c.mino.MakeRPC("cosi", newHandler(c, r), factory)
	if err != nil {
		return nil, xerrors.Errorf("couldn't make rpc: %v", err)
	}

	actor := thresholdActor{
		CoSi:    c,
		logger:  dela.Logger.With().Str("addr", c.mino.GetAddress().String()).Logger(),
		me:      c.mino.GetAddress(),
		rpc:     rpc,
		reactor: r,
	}

	return actor, nil
}
