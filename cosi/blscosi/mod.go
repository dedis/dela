package blscosi

import (
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/cosi"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/crypto/bls"
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/kyber/v3/pairing"
	"go.dedis.ch/kyber/v3/util/key"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

var suite = pairing.NewSuiteBn256()

// NewSigner returns a signer compatible with this implementation of cosi.
func NewSigner() crypto.AggregateSigner {
	kp := key.NewKeyPair(suite)
	return bls.NewSigner(kp)
}

// BlsCoSi is an implementation of the collective signing interface by
// using BLS signatures.
type BlsCoSi struct {
	mino   mino.Mino
	rpc    mino.RPC
	signer crypto.AggregateSigner
}

// NewBlsCoSi returns a new collective signing instance.
func NewBlsCoSi(o mino.Mino, signer crypto.AggregateSigner) *BlsCoSi {
	return &BlsCoSi{
		mino:   o,
		signer: signer,
	}
}

// GetPublicKey returns the public key for this instance.
func (cosi *BlsCoSi) GetPublicKey() crypto.PublicKey {
	return cosi.signer.PublicKey()
}

// GetVerifier returns a verifier that can be used to verify signatures
// from this collective signing.
func (cosi *BlsCoSi) GetVerifier() crypto.Verifier {
	return bls.NewVerifier()
}

// Listen starts the RPC that will handle signing requests.
func (cosi *BlsCoSi) Listen(h cosi.Hashable) error {
	rpc, err := cosi.mino.MakeRPC("cosi", newHandler(cosi.signer, h))
	if err != nil {
		return err
	}

	cosi.rpc = rpc

	return nil
}

// Sign returns the collective signature of the block.
func (cosi *BlsCoSi) Sign(msg proto.Message, signers ...cosi.Cosigner) (crypto.Signature, error) {
	if cosi.rpc == nil {
		return nil, xerrors.New("cosi is not listening")
	}

	data, err := ptypes.MarshalAny(msg)
	if err != nil {
		return nil, err
	}

	nodes := make([]mino.Node, len(signers))
	for i, signer := range signers {
		nodes[i] = signer
	}

	// TODO: Address interface to inline ?
	msgs, errs := cosi.rpc.Call(&SignatureRequest{Message: data}, nodes...)

	var agg crypto.Signature
	for {
		select {
		case resp, ok := <-msgs:
			if !ok {
				// TODO: verify signature
				fabric.Logger.Trace().Msgf("Closing")
				return agg, nil
			}

			fabric.Logger.Trace().Msgf("Response: %+v", resp)

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
		}
	}
}
