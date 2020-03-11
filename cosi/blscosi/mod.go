package blscosi

import (
	"github.com/golang/protobuf/ptypes"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/cosi"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/mino"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

// BlsCoSi is an implementation of the collective signing interface by
// using BLS signatures.
type BlsCoSi struct {
	mino   mino.Mino
	rpc    mino.RPC
	signer crypto.AggregateSigner
	hasher cosi.Hashable
}

// NewBlsCoSi returns a new collective signing instance.
func NewBlsCoSi(o mino.Mino, signer crypto.AggregateSigner) *BlsCoSi {
	return &BlsCoSi{
		mino:   o,
		signer: signer,
	}
}

// GetPublicKeyFactory returns the public key factory.
func (cosi *BlsCoSi) GetPublicKeyFactory() crypto.PublicKeyFactory {
	return cosi.signer.GetPublicKeyFactory()
}

// GetSignatureFactory returns the signature factory.
func (cosi *BlsCoSi) GetSignatureFactory() crypto.SignatureFactory {
	return cosi.signer.GetSignatureFactory()
}

// GetVerifier returns a verifier that can be used to verify signatures
// from this collective signing.
func (cosi *BlsCoSi) GetVerifier(ca cosi.CollectiveAuthority) crypto.Verifier {
	pubkeys := make([]crypto.PublicKey, 0, ca.Len())
	if ca != nil {
		iter := ca.PublicKeyIterator()
		for iter.HasNext() {
			pubkeys = append(pubkeys, iter.GetNext())
		}
	}

	return cosi.signer.GetVerifierFactory().Create(pubkeys)
}

// Listen starts the RPC that will handle signing requests.
func (cosi *BlsCoSi) Listen(h cosi.Hashable) error {
	cosi.hasher = h

	rpc, err := cosi.mino.MakeRPC("cosi", newHandler(cosi.signer, h))
	if err != nil {
		return err
	}

	cosi.rpc = rpc

	return nil
}

// Sign returns the collective signature of the block.
func (cosi *BlsCoSi) Sign(msg cosi.Message, ca cosi.CollectiveAuthority) (crypto.Signature, error) {
	if cosi.rpc == nil {
		return nil, xerrors.New("cosi is not listening")
	}

	packed, err := msg.Pack()
	if err != nil {
		return nil, encoding.NewEncodingError("message", err)
	}

	data, err := ptypes.MarshalAny(packed)
	if err != nil {
		return nil, err
	}

	pubkeys := make([]crypto.PublicKey, 0, ca.Len())
	iter := ca.PublicKeyIterator()
	for iter.HasNext() {
		pubkeys = append(pubkeys, iter.GetNext())
	}

	verifier := cosi.signer.GetVerifierFactory().Create(pubkeys)

	msgs, errs := cosi.rpc.Call(&SignatureRequest{Message: data}, ca)

	var agg crypto.Signature
	for {
		select {
		case resp, ok := <-msgs:
			if !ok {
				err = verifier.Verify(msg.GetHash(), agg)
				if err != nil {
					return nil, xerrors.Errorf("couldn't verify the aggregation: %v", err)
				}

				return agg, nil
			}

			reply := resp.(*SignatureResponse)
			sig, err := cosi.signer.GetSignatureFactory().FromProto(reply.GetSignature())
			if err != nil {
				return nil, err
			}

			if agg == nil {
				agg = sig
			} else {
				agg, err = cosi.signer.Aggregate(agg, sig)
				if err != nil {
					return nil, err
				}
			}
		case err := <-errs:
			fabric.Logger.Err(err).Msg("Error during collective signing")
			return nil, err
		}
	}
}
