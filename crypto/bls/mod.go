package bls

import (
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/pairing"
	"go.dedis.ch/kyber/v3/sign/bls"
	"go.dedis.ch/kyber/v3/util/key"
	"golang.org/x/xerrors"
)

//go:generate protoc -I ./ --go_out=./ ./messages.proto

var suite = pairing.NewSuiteBn256()

type publicKey struct {
	point kyber.Point
}

func (pk publicKey) MarshalBinary() ([]byte, error) {
	return pk.point.MarshalBinary()
}

func (pk publicKey) Pack() (proto.Message, error) {
	buffer, err := pk.point.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return &PublicKeyProto{Data: buffer}, nil
}

type signature struct {
	data []byte
}

func (sig signature) MarshalBinary() ([]byte, error) {
	return sig.data, nil
}

func (sig signature) Pack() (proto.Message, error) {
	return &SignatureProto{Data: sig.data}, nil
}

type publicKeyFactory struct{}

func (f publicKeyFactory) fromAny(src *any.Any) (crypto.PublicKey, error) {
	var pkp PublicKeyProto
	err := ptypes.UnmarshalAny(src, &pkp)
	if err != nil {
		return nil, err
	}

	return f.FromProto(&pkp)
}

func (f publicKeyFactory) FromProto(src proto.Message) (crypto.PublicKey, error) {
	switch msg := src.(type) {
	case *PublicKeyProto:
		point := suite.Point()
		err := point.UnmarshalBinary(msg.GetData())
		if err != nil {
			return nil, xerrors.Errorf("failed to unmarshal msg to point: %v", err)
		}

		return publicKey{point: point}, nil
	case *any.Any:
		return f.fromAny(msg)
	default:
		return nil, xerrors.Errorf("invalid public key type '%v'", msg)
	}
}

type signatureFactory struct{}

func (f signatureFactory) FromAny(src *any.Any) (crypto.Signature, error) {
	var sigproto SignatureProto
	err := ptypes.UnmarshalAny(src, &sigproto)
	if err != nil {
		return nil, err
	}

	return f.FromProto(&sigproto)
}

func (f signatureFactory) FromProto(src proto.Message) (crypto.Signature, error) {
	switch msg := src.(type) {
	case *SignatureProto:
		return signature{data: msg.GetData()}, nil
	case *any.Any:
		return f.FromAny(msg)
	default:
		return nil, xerrors.Errorf("invalid signature type '%v'", msg)
	}
}

// verifier implements the verifier interface for BLS.
type verifier struct{}

// NewVerifier returns a new verifier that can verify BLS signatures.
func NewVerifier() crypto.Verifier {
	return verifier{}
}

// GetPublicKeyFactory returns a factory to make BLS public keys.
func (v verifier) GetPublicKeyFactory() crypto.PublicKeyFactory {
	return publicKeyFactory{}
}

// GetSignatureFactory returns a factory to make BLS signatures.
func (v verifier) GetSignatureFactory() crypto.SignatureFactory {
	return signatureFactory{}
}

// Verify returns no error if the signature matches the message.
func (v verifier) Verify(pubkeys []crypto.PublicKey, msg []byte, sig crypto.Signature) error {
	points := make([]kyber.Point, len(pubkeys))
	for i, pubkey := range pubkeys {
		points[i] = pubkey.(publicKey).point
	}

	aggKey := bls.AggregatePublicKeys(suite, points...)

	err := bls.Verify(suite, aggKey, msg, sig.(signature).data)
	if err != nil {
		return err
	}

	return nil
}

type signer struct {
	verifier

	keyPair *key.Pair
}

// NewSigner returns a new BLS signer. It supports aggregation.
func NewSigner(kp *key.Pair) crypto.AggregateSigner {
	return signer{keyPair: kp}
}

func (s signer) PublicKey() crypto.PublicKey {
	return publicKey{point: s.keyPair.Public}
}

func (s signer) Sign(msg []byte) (crypto.Signature, error) {
	sig, err := bls.Sign(suite, s.keyPair.Private, msg)
	if err != nil {
		return nil, err
	}

	return signature{data: sig}, nil
}

func (s signer) Aggregate(signatures ...crypto.Signature) (crypto.Signature, error) {
	buffers := make([][]byte, len(signatures))
	for i, sig := range signatures {
		buffers[i] = sig.(signature).data
	}

	agg, err := bls.AggregateSignatures(suite, buffers...)
	if err != nil {
		return nil, err
	}

	return signature{data: agg}, nil
}
