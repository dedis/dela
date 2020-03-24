package flatcosi

import (
	"context"

	"github.com/golang/protobuf/proto"
	"github.com/rs/zerolog"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/cosi"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

const (
	rpcName = "cosi"
)

var protoenc encoding.ProtoMarshaler = encoding.NewProtoEncoder()

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

// GetPublicKeyFactory returns the public key factory.
func (cosi *Flat) GetPublicKeyFactory() crypto.PublicKeyFactory {
	return cosi.signer.GetPublicKeyFactory()
}

// GetSignatureFactory returns the signature factory.
func (cosi *Flat) GetSignatureFactory() crypto.SignatureFactory {
	return cosi.signer.GetSignatureFactory()
}

// GetVerifier returns a verifier that can be used to verify signatures
// from this collective authority.
func (cosi *Flat) GetVerifier(ca cosi.CollectiveAuthority) (crypto.Verifier, error) {
	if ca == nil {
		return nil, xerrors.New("collective authority is nil")
	}

	verifier, err := cosi.signer.GetVerifierFactory().FromIterator(ca.PublicKeyIterator())
	if err != nil {
		return nil, xerrors.Errorf("couldn't create verifier: %v", err)
	}

	return verifier, nil
}

// Listen creates an actor that starts an RPC called cosi and respond to signing
// requests. The actor can also be used to sign a message.
func (cosi *Flat) Listen(h cosi.Hashable) (cosi.Actor, error) {
	actor := flatActor{
		logger: fabric.Logger,
		signer: cosi.signer,
	}

	rpc, err := cosi.mino.MakeRPC(rpcName, newHandler(cosi.signer, h))
	if err != nil {
		return nil, xerrors.Errorf("couldn't make the rpc: %v", err)
	}

	actor.rpc = rpc

	return actor, nil
}

type flatActor struct {
	logger zerolog.Logger
	rpc    mino.RPC
	signer crypto.AggregateSigner
}

// Sign returns the collective signature of the block.
func (a flatActor) Sign(ctx context.Context, msg cosi.Message,
	ca cosi.CollectiveAuthority) (crypto.Signature, error) {

	packed, err := msg.Pack()
	if err != nil {
		return nil, encoding.NewEncodingError("message", err)
	}

	data, err := protoenc.MarshalAny(packed)
	if err != nil {
		return nil, encoding.NewAnyEncodingError(packed, err)
	}

	verifier, err := a.signer.GetVerifierFactory().FromIterator(ca.PublicKeyIterator())
	if err != nil {
		return nil, xerrors.Errorf("couldn't make verifier: %v", err)
	}

	msgs, errs := a.rpc.Call(ctx, &SignatureRequest{Message: data}, ca)

	var agg crypto.Signature
	for {
		select {
		case resp, ok := <-msgs:
			if !ok {
				if agg == nil {
					return nil, xerrors.New("signature is nil")
				}

				err = verifier.Verify(msg.GetHash(), agg)
				if err != nil {
					return nil, xerrors.Errorf("couldn't verify the aggregation: %v", err)
				}

				return agg, nil
			}

			agg, err = a.processResponse(resp, agg)
			if err != nil {
				// Keep the protocol going if an error occurred so that a bad
				// player cannot intentionally stop the protocol.
				a.logger.Err(err).Msg("error when processing response")
			}
		case err := <-errs:
			// Keep the protocol going if a request message fails to be transmitted.
			a.logger.Err(err).Msg("error during collective signing")
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
		return nil, encoding.NewDecodingError("signature", err)
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
