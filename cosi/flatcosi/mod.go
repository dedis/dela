// Package flatcosi is a flat implementation of a collective signing so that the
// orchestrator will contact all the participants to require their signatures
// and then aggregate them to the final one.
//
// Documentation Last Review: 05.10.2020
//
package flatcosi

import (
	"context"

	"github.com/rs/zerolog"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

const (
	rpcName = "cosi"
)

// Flat is an implementation of the collective signing interface by
// using BLS signatures. It ignores the threshold and always requests a
// signature from every participant.
//
// - implements cosi.CollectiveSigning
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

// GetPublicKeyFactory implements cosi.CollectiveSigning. It returns the public
// key factory.
func (flat *Flat) GetPublicKeyFactory() crypto.PublicKeyFactory {
	return flat.signer.GetPublicKeyFactory()
}

// GetSignatureFactory implements cosi.CollectiveSigning. It returns the
// signature factory.
func (flat *Flat) GetSignatureFactory() crypto.SignatureFactory {
	return flat.signer.GetSignatureFactory()
}

// GetVerifierFactory implements cosi.CollectiveSigning. It returns the verifier
// factory.
func (flat *Flat) GetVerifierFactory() crypto.VerifierFactory {
	return flat.signer.GetVerifierFactory()
}

// SetThreshold implements cosi.CollectiveSigning. It ignores the new threshold
// as this implementation only accepts full participation.
func (flat *Flat) SetThreshold(fn cosi.Threshold) {}

// Listen implements cosi.CollectiveSigning. It creates an actor that starts an
// RPC called cosi and respond to signing requests. The actor can also be used
// to sign a message.
func (flat *Flat) Listen(r cosi.Reactor) (cosi.Actor, error) {
	actor := flatActor{
		logger:  dela.Logger,
		me:      flat.mino.GetAddress(),
		signer:  flat.signer,
		reactor: r,
	}

	factory := cosi.NewMessageFactory(r, flat.signer.GetSignatureFactory())

	actor.rpc = mino.MustCreateRPC(flat.mino, rpcName, newHandler(flat.signer, r), factory)

	return actor, nil
}

// FlatActor is the active component of the flat collective signing. It provides
// a primitive to trigger a request for signatures from the participants.
//
// - implements cosi.Actor
type flatActor struct {
	logger  zerolog.Logger
	me      mino.Address
	rpc     mino.RPC
	signer  crypto.AggregateSigner
	reactor cosi.Reactor
}

// Sign implements cosi.Actor. It returns the collective signature of the
// message if every participant returns its signature.
func (a flatActor) Sign(ctx context.Context, msg serde.Message,
	ca crypto.CollectiveAuthority) (crypto.Signature, error) {

	verifier, err := a.signer.GetVerifierFactory().FromAuthority(ca)
	if err != nil {
		return nil, xerrors.Errorf("couldn't make verifier: %v", err)
	}

	req := cosi.SignatureRequest{
		Value: msg,
	}

	msgs, err := a.rpc.Call(ctx, req, ca)
	if err != nil {
		return nil, xerrors.Errorf("call aborted: %v", err)
	}

	digest, err := a.reactor.Invoke(a.me, msg)
	if err != nil {
		return nil, xerrors.Errorf("couldn't react to message: %v", err)
	}

	var agg crypto.Signature
	for {
		resp, more := <-msgs
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

		reply, err := resp.GetMessageOrError()
		if err != nil {
			return nil, xerrors.Errorf("one request has failed: %v", err)
		}

		agg, err = a.processResponse(reply, agg)
		if err != nil {
			return nil, xerrors.Errorf("couldn't process response: %v", err)
		}
	}
}

func (a flatActor) processResponse(resp serde.Message, agg crypto.Signature) (crypto.Signature, error) {
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
