package threshold

import (
	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

// Threshold is a function that returns the threshold to reach for a given n.
type Threshold func(int) int

func defaultThreshold(n int) int {
	return n
}

// CoSi is an implementation of the cosi.CollectiveSigning interface that is
// using streams to parallelize the work.
type CoSi struct {
	mino   mino.Mino
	signer crypto.AggregateSigner

	Threshold Threshold
}

// NewCoSi returns a new instance.
func NewCoSi(m mino.Mino, signer crypto.AggregateSigner) *CoSi {
	return &CoSi{
		mino:      m,
		signer:    signer,
		Threshold: defaultThreshold,
	}
}

// GetSigner implements cosi.CollectiveSigning. It returns the signer of the
// instance.
func (c *CoSi) GetSigner() crypto.Signer {
	return c.signer
}

// GetPublicKeyFactory implements cosi.CollectiveSigning. It returns the public
// key factory.
func (c *CoSi) GetPublicKeyFactory() serde.Factory {
	return c.signer.GetPublicKeyFactory()
}

// GetSignatureFactory implements cosi.CollectiveSigning. It returns the
// signature factory.
func (c *CoSi) GetSignatureFactory() serde.Factory {
	return signatureFactory{
		sigFactory: c.signer.GetSignatureFactory(),
	}
}

// GetVerifierFactory implements cosi.CollectiveSigning. It returns the verifier
// factory.
func (c *CoSi) GetVerifierFactory() crypto.VerifierFactory {
	return verifierFactory{factory: c.signer.GetVerifierFactory()}
}

// Listen implements cosi.CollectiveSigning.
func (c *CoSi) Listen(r cosi.Reactor) (cosi.Actor, error) {
	rpc, err := c.mino.MakeRPC("cosi", newHandler(c, r))
	if err != nil {
		return nil, xerrors.Errorf("couldn't make rpc: %v", err)
	}

	actor := thresholdActor{
		CoSi:    c,
		me:      c.mino.GetAddress(),
		rpc:     rpc,
		reactor: r,
	}

	return actor, nil
}
