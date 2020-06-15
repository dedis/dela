package bls

import (
	"bytes"
	"fmt"

	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/common/json"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/pairing"
	"go.dedis.ch/kyber/v3/sign/bls"
	"go.dedis.ch/kyber/v3/util/key"
	"golang.org/x/xerrors"
)

const (
	// Algorithm is the name of the curve used for the BLS signature.
	Algorithm = "CURVE-BN256"
)

var (
	suite = pairing.NewSuiteBn256()
)

// publicKey can be provided to verify a BLS signature.
type publicKey struct {
	serde.UnimplementedMessage

	point kyber.Point
}

// MarshalBinary implements encoding.BinaryMarshaler. It produces a slice of
// bytes representing the public key.
func (pk publicKey) MarshalBinary() ([]byte, error) {
	return pk.point.MarshalBinary()
}

// VisitJSON implements serde.Message. It returns the JSON message for the
// public key.
func (pk publicKey) VisitJSON(serde.Serializer) (interface{}, error) {
	buffer, err := pk.point.MarshalBinary()
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal point: %v", err)
	}

	m := json.PublicKey{
		Algorithm: json.Algorithm{Name: Algorithm},
		Data:      buffer,
	}

	return m, nil
}

// Verify implements crypto.PublicKey. It returns nil if the signature matches
// the message with this public key.
func (pk publicKey) Verify(msg []byte, sig crypto.Signature) error {
	signature, ok := sig.(signature)
	if !ok {
		return xerrors.Errorf("invalid signature type '%T'", sig)
	}

	err := bls.Verify(suite, pk.point, msg, signature.data)
	if err != nil {
		return xerrors.Errorf("bls verify failed: %v", err)
	}

	return nil
}

// Equal implements crypto.PublicKey. It returns true if the other public key
// is the same.
func (pk publicKey) Equal(other crypto.PublicKey) bool {
	pubkey, ok := other.(publicKey)
	if !ok {
		return false
	}

	return pubkey.point.Equal(pk.point)
}

// MarshalText implements encoding.TextMarshaler. It returns a text
// representation of the public key.
func (pk publicKey) MarshalText() ([]byte, error) {
	buffer, err := pk.MarshalBinary()
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal: %v", err)
	}

	return []byte(fmt.Sprintf("bls:%x", buffer)), nil
}

// String implements fmt.String. It returns a string representation of the
// point.
func (pk publicKey) String() string {
	buffer, err := pk.MarshalText()
	if err != nil {
		return "bls:malformed_point"
	}

	// Output only the prefix and 16 characters of the buffer in hexadecimal.
	return string(buffer)[:4+16]
}

// signature is a proof of the integrity of a single message associated with a
// unique public key.
type signature struct {
	serde.UnimplementedMessage

	data []byte
}

// MarshalBinary implements encoding.BinaryMarshaler. It returns a slice of
// bytes representing the signature.
func (sig signature) MarshalBinary() ([]byte, error) {
	return sig.data, nil
}

// VisitJSON implements serde.Message. It returns the JSON message for the
// signature.
func (sig signature) VisitJSON(serde.Serializer) (interface{}, error) {
	m := json.Signature{
		Algorithm: json.Algorithm{Name: Algorithm},
		Data:      sig.data,
	}

	return m, nil
}

// Equal implements crypto.PublicKey.
func (sig signature) Equal(other crypto.Signature) bool {
	otherSig, ok := other.(signature)
	if !ok {
		return false
	}

	return bytes.Equal(sig.data, otherSig.data)
}

// publicKeyFactory creates BLS compatible public key from protobuf messages.
type publicKeyFactory struct {
	serde.UnimplementedFactory
}

// NewPublicKeyFactory returns a new instance of the factory.
func NewPublicKeyFactory() serde.Factory {
	return publicKeyFactory{}
}

// VisitJSON implements serde.Factory. It deserializes the public key in JSON
// format.
func (f publicKeyFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.PublicKey{}
	err := in.Feed(&m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize data: %v", err)
	}

	point := suite.Point()
	err = point.UnmarshalBinary(m.Data)
	if err != nil {
		return nil, xerrors.Errorf("couldn't unmarshal point: %v", err)
	}

	return publicKey{point: point}, nil
}

// signatureFactory provides functions to create BLS signatures from protobuf
// messages.
type signatureFactory struct {
	serde.UnimplementedFactory
}

// NewSignatureFactory returns a new instance of the factory.
func NewSignatureFactory() serde.Factory {
	return signatureFactory{}
}

// VisitJSON implements serde.Factory. It deserializes the signature in JSON
// format.
func (f signatureFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.Signature{}
	err := in.Feed(&m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize data: %v", err)
	}

	return signature{data: m.Data}, nil
}

// verifier provides primitives to verify a BLS signature of a unique message.
type blsVerifier struct {
	points []kyber.Point
}

// NewVerifier returns a new verifier that can verify BLS signatures.
func newVerifier(points []kyber.Point) crypto.Verifier {
	return blsVerifier{points: points}
}

// Verify implements crypto.Verifier. It returns nil if the signature matches
// the message, or an error otherwise.
func (v blsVerifier) Verify(msg []byte, sig crypto.Signature) error {
	aggKey := bls.AggregatePublicKeys(suite, v.points...)

	err := bls.Verify(suite, aggKey, msg, sig.(signature).data)
	if err != nil {
		return err
	}

	return nil
}

type verifierFactory struct{}

// FromIterator implements crypto.VerifierFactory. It returns a verifier that
// will verify the signatures collectively signed by all the signers associated
// with the public keys.
func (v verifierFactory) FromAuthority(ca crypto.CollectiveAuthority) (crypto.Verifier, error) {
	if ca == nil {
		return nil, xerrors.New("authority is nil")
	}

	points := make([]kyber.Point, 0, ca.Len())
	iter := ca.PublicKeyIterator()
	for iter.HasNext() {
		next := iter.GetNext()
		pk, ok := next.(publicKey)
		if !ok {
			return nil, xerrors.Errorf("invalid public key type: %T", next)
		}

		points = append(points, pk.point)
	}

	return newVerifier(points), nil
}

// FromArray implements crypto.VerifierFactory. It returns a verifier that will
// verify the signatures collectively signed by all the signers associated with
// the public keys.
func (v verifierFactory) FromArray(publicKeys []crypto.PublicKey) (crypto.Verifier, error) {
	points := make([]kyber.Point, len(publicKeys))
	for i, pubkey := range publicKeys {
		pk, ok := pubkey.(publicKey)
		if !ok {
			return nil, xerrors.Errorf("invalid public key type: %T", pubkey)
		}

		points[i] = pk.point
	}

	return newVerifier(points), nil
}

type signer struct {
	keyPair *key.Pair
}

// NewSigner returns a new random BLS signer that supports aggregation.
func NewSigner() crypto.AggregateSigner {
	kp := key.NewKeyPair(suite)
	return signer{
		keyPair: kp,
	}
}

// GetVerifierFactory implements crypto.Signer. It returns the verifier factory
// for BLS signatures.
func (s signer) GetVerifierFactory() crypto.VerifierFactory {
	return verifierFactory{}
}

// GetPublicKeyFactory implements crypto.Signer. It returns the public key
// factory for BLS signatures.
func (s signer) GetPublicKeyFactory() serde.Factory {
	return publicKeyFactory{}
}

// GetSignatureFactory implements crypto.Signer. It returns the signature
// factory for BLS signatures.
func (s signer) GetSignatureFactory() serde.Factory {
	return signatureFactory{}
}

// GetPublicKey implements crypto.Signer. It returns the public key of the
// signer that can be used to verify signatures.
func (s signer) GetPublicKey() crypto.PublicKey {
	return publicKey{point: s.keyPair.Public}
}

// Sign implements crypto.Signer. It signs the message in parameter and returns
// the signature, or an error if it cannot sign.
func (s signer) Sign(msg []byte) (crypto.Signature, error) {
	sig, err := bls.Sign(suite, s.keyPair.Private, msg)
	if err != nil {
		return nil, xerrors.Errorf("couldn't make bls signature: %v", err)
	}

	return signature{data: sig}, nil
}

// Aggregate implements crypto.Signer. It aggregates the signatures into a
// single one that can be verifier with the aggregated public key associated.
func (s signer) Aggregate(signatures ...crypto.Signature) (crypto.Signature, error) {
	buffers := make([][]byte, len(signatures))
	for i, sig := range signatures {
		buffers[i] = sig.(signature).data
	}

	agg, err := bls.AggregateSignatures(suite, buffers...)
	if err != nil {
		return nil, xerrors.Errorf("couldn't aggregate: %v", err)
	}

	return signature{data: agg}, nil
}
