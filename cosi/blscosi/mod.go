package blscosi

import (
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"go.dedis.ch/kyber/v3/pairing"
	"go.dedis.ch/kyber/v3/util/key"
	"go.dedis.ch/m"
	"go.dedis.ch/m/blockchain"
	"go.dedis.ch/m/crypto"
	"go.dedis.ch/m/crypto/bls"
	"go.dedis.ch/m/mino"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

var suite = pairing.NewSuiteBn256()

// NewSigner returns a signer compatible with this implementation of cosi.
func NewSigner() crypto.AggregateSigner {
	kp := key.NewKeyPair(suite)
	return bls.NewSigner(kp)
}

// Validator is the interface that is used to validate a block.
type Validator interface {
	Validate(msg proto.Message) ([]byte, error)
}

// BlsCoSi is an implementation of the collective signing interface by
// using BLS signatures.
type BlsCoSi struct {
	rpc    mino.RPC
	signer crypto.AggregateSigner
}

// NewBlsCoSi returns a new collective signing instance.
func NewBlsCoSi(o mino.Mino, signer crypto.AggregateSigner, v Validator) (*BlsCoSi, error) {
	rpc, err := o.MakeRPC("cosi", newHandler(o, signer, v))
	if err != nil {
		return nil, err
	}

	return &BlsCoSi{
		rpc:    rpc,
		signer: signer,
	}, nil
}

// PublicKey returns the public key for this instance.
func (cosi *BlsCoSi) PublicKey() crypto.PublicKey {
	return cosi.signer.PublicKey()
}

// Sign returns the collective signature of the block.
func (cosi *BlsCoSi) Sign(ro blockchain.Roster, msg proto.Message) (crypto.Signature, error) {
	data, err := ptypes.MarshalAny(msg)
	if err != nil {
		return nil, err
	}

	addrs := ro.GetAddresses()

	m.Logger.Trace().Msgf("Roster %v", addrs)
	msgs, errs := cosi.rpc.Call(&SignatureRequest{Message: data}, addrs...)

	var agg crypto.Signature
	for {
		select {
		case resp, ok := <-msgs:
			if !ok {
				// TODO: verify signature
				m.Logger.Trace().Msgf("Closing")
				return agg, nil
			}

			m.Logger.Trace().Msgf("Response: %+v", resp)

			reply := resp.(*SignatureResponse)
			sig, err := cosi.signer.GetSignatureFactory().FromAny(reply.GetSignature())
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
			m.Logger.Err(err).Msg("Error during collective signing")
		}
	}
}

// MakeVerifier returns a verifier that can be used to verify signatures
// from this collective signing.
func (cosi *BlsCoSi) MakeVerifier() crypto.Verifier {
	return bls.NewVerifier()
}
