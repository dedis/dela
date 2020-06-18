package flatcosi

import (
	"context"

	"github.com/rs/zerolog"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serdeng"
	"golang.org/x/xerrors"
)

const (
	rpcName = "cosi"
)

// Flat is an implementation of the collective signing interface by
// using BLS signatures.
type Flat struct {
	mino   mino.Mino
	signer crypto.AggregateSigner
}

// NewFlat returns a new collective signing instance.
func NewFlat(o mino.Mino, signer crypto.AggregateSigner) *Flat {
	return &Flat{
		mino:   o,
		signer: signer,
	}
}

// GetSigner implements cosi.CollectiveSigning. It returns the signer of the
// instance.
func (flat *Flat) GetSigner() crypto.Signer {
	return flat.signer
}

// GetPublicKeyFactory returns the public key factory.
func (flat *Flat) GetPublicKeyFactory() crypto.PublicKeyFactory {
	return flat.signer.GetPublicKeyFactory()
}

// GetSignatureFactory returns the signature factory.
func (flat *Flat) GetSignatureFactory() crypto.SignatureFactory {
	return flat.signer.GetSignatureFactory()
}

// GetVerifierFactory returns the verifier factory.
func (flat *Flat) GetVerifierFactory() crypto.VerifierFactory {
	return flat.signer.GetVerifierFactory()
}

// Listen creates an actor that starts an RPC called cosi and respond to signing
// requests. The actor can also be used to sign a message.
func (flat *Flat) Listen(r cosi.Reactor) (cosi.Actor, error) {
	actor := flatActor{
		logger:  dela.Logger,
		me:      flat.mino.GetAddress(),
		signer:  flat.signer,
		reactor: r,
	}

	factory := cosi.NewMessageFactory(r, flat.signer.GetSignatureFactory())

	rpc, err := flat.mino.MakeRPC(rpcName, newHandler(flat.signer, r), factory)
	if err != nil {
		return nil, xerrors.Errorf("couldn't make the rpc: %v", err)
	}

	actor.rpc = rpc

	return actor, nil
}

type flatActor struct {
	logger  zerolog.Logger
	me      mino.Address
	rpc     mino.RPC
	signer  crypto.AggregateSigner
	reactor cosi.Reactor
}

// Sign returns the collective signature of the block.
func (a flatActor) Sign(ctx context.Context, msg serdeng.Message,
	ca crypto.CollectiveAuthority) (crypto.Signature, error) {

	verifier, err := a.signer.GetVerifierFactory().FromAuthority(ca)
	if err != nil {
		return nil, xerrors.Errorf("couldn't make verifier: %v", err)
	}

	req := cosi.SignatureRequest{
		Value: msg,
	}

	msgs, errs := a.rpc.Call(ctx, req, ca)

	digest, err := a.reactor.Invoke(a.me, msg)
	if err != nil {
		return nil, xerrors.Errorf("couldn't react to message: %v", err)
	}

	var agg crypto.Signature
	for {
		select {
		case resp, more := <-msgs:
			if !more {
				if agg == nil {
					return nil, xerrors.New("signature is nil")
				}

				err = verifier.Verify(digest, agg)
				if err != nil {
					return nil, xerrors.Errorf("couldn't verify the aggregation: %v", err)
				}

				return agg, nil
			}

			agg, err = a.processResponse(resp, agg)
			if err != nil {
				return nil, xerrors.Errorf("couldn't process response: %v", err)
			}
		case err := <-errs:
			return nil, xerrors.Errorf("one request has failed: %v", err)
		}
	}
}

func (a flatActor) processResponse(resp serdeng.Message, agg crypto.Signature) (crypto.Signature, error) {
	reply, ok := resp.(cosi.SignatureResponse)
	if !ok {
		return nil, xerrors.Errorf("invalid response type '%T'", resp)
	}

	var err error

	if agg == nil {
		agg = reply.Signature
	} else {
		agg, err = a.signer.Aggregate(agg, reply.Signature)
		if err != nil {
			return nil, xerrors.Errorf("couldn't aggregate: %v", err)
		}
	}

	return agg, nil
}
