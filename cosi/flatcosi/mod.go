package flatcosi

import (
	"context"

	"github.com/golang/protobuf/proto"
	"github.com/rs/zerolog"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/encoding"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

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
func (cosi *Flat) GetSigner() crypto.Signer {
	return cosi.signer
}

// GetPublicKeyFactory returns the public key factory.
func (cosi *Flat) GetPublicKeyFactory() crypto.PublicKeyFactory {
	return cosi.signer.GetPublicKeyFactory()
}

// GetSignatureFactory returns the signature factory.
func (cosi *Flat) GetSignatureFactory() crypto.SignatureFactory {
	return cosi.signer.GetSignatureFactory()
}

// GetVerifierFactory returns the verifier factory.
func (cosi *Flat) GetVerifierFactory() crypto.VerifierFactory {
	return cosi.signer.GetVerifierFactory()
}

// Listen creates an actor that starts an RPC called cosi and respond to signing
// requests. The actor can also be used to sign a message.
func (cosi *Flat) Listen(r cosi.Reactor) (cosi.Actor, error) {
	actor := flatActor{
		logger:  dela.Logger,
		me:      cosi.mino.GetAddress(),
		signer:  cosi.signer,
		encoder: encoding.NewProtoEncoder(),
		reactor: r,
	}

	rpc, err := cosi.mino.MakeRPC(rpcName, newHandler(cosi.signer, r))
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
	encoder encoding.ProtoMarshaler
	reactor cosi.Reactor
}

// Sign returns the collective signature of the block.
func (a flatActor) Sign(ctx context.Context, msg encoding.Packable,
	ca crypto.CollectiveAuthority) (crypto.Signature, error) {

	data, err := a.encoder.PackAny(msg)
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack message: %v", err)
	}

	verifier, err := a.signer.GetVerifierFactory().FromAuthority(ca)
	if err != nil {
		return nil, xerrors.Errorf("couldn't make verifier: %v", err)
	}

	msgs, errs := a.rpc.Call(ctx, &SignatureRequest{Message: data}, ca)

	digest, err := a.reactor.Invoke(a.me, data)
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

func (a flatActor) processResponse(resp proto.Message, agg crypto.Signature) (crypto.Signature, error) {
	reply, ok := resp.(*SignatureResponse)
	if !ok {
		return nil, xerrors.Errorf("response type is invalid: %T", resp)
	}

	sig, err := a.signer.GetSignatureFactory().FromProto(reply.GetSignature())
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode signature: %v", err)
	}

	if agg == nil {
		agg = sig
	} else {
		agg, err = a.signer.Aggregate(agg, sig)
		if err != nil {
			return nil, xerrors.Errorf("couldn't aggregate: %v", err)
		}
	}

	return agg, nil
}
