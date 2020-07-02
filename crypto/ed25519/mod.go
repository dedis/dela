package ed25519

import (
	"bytes"
	"fmt"

	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/common/json"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/sign/schnorr"
	"go.dedis.ch/kyber/v3/suites"
	"go.dedis.ch/kyber/v3/util/key"
	"golang.org/x/xerrors"
)

const (
	// Algorithm is the name of the curve used for the schnorr signature.
	Algorithm = "CURVE-ED25519"
)

var (
	suite = suites.MustFind("Ed25519")
)

// PublicKey can be provided to verify a schnorr signature.
type PublicKey struct {
	serde.UnimplementedMessage

	point kyber.Point
}

// MarshalBinary implements encoding.BinaryMarshaler. It produces a slice of
// bytes representing the public key.
func (pk PublicKey) MarshalBinary() ([]byte, error) {
	return pk.point.MarshalBinary()
}

// VisitJSON implements serde.Message. It returns the JSON message for the
// public key.
func (pk PublicKey) VisitJSON(serde.Serializer) (interface{}, error) {
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
func (pk PublicKey) Verify(msg []byte, sig crypto.Signature) error {
	signature, ok := sig.(signature)
	if !ok {
		return xerrors.Errorf("invalid signature type '%T'", sig)
	}

	err := schnorr.Verify(suite, pk.point, msg, signature.data)
	if err != nil {
		return xerrors.Errorf("schnorr verify failed: %v", err)
	}

	return nil
}

// Equal implements crypto.PublicKey. It returns true if the other public key
// is the same.
func (pk PublicKey) Equal(other crypto.PublicKey) bool {
	pubkey, ok := other.(PublicKey)
	if !ok {
		return false
	}

	return pubkey.point.Equal(pk.point)
}

// MarshalText implements encoding.TextMarshaler. It returns a text
// representation of the public key.
func (pk PublicKey) MarshalText() ([]byte, error) {
	buffer, err := pk.MarshalBinary()
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal: %v", err)
	}

	return []byte(fmt.Sprintf("schnorr:%x", buffer)), nil
}

// GetPoint returns the kyber.point
func (pk PublicKey) GetPoint() kyber.Point {
	return pk.point
}

// String implements fmt.String. It returns a string representation of the
// point.
func (pk PublicKey) String() string {
	buffer, err := pk.MarshalText()
	if err != nil {
		return "schnorr:malformed_point"
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

// publicKeyFactory creates schnorr compatible public key from protobuf
// messages.
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

	return PublicKey{point: point}, nil
}

// signatureFactory provides functions to create schnorr signatures from
// protobuf messages.
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

// verifier provides primitives to verify a schnorr signature of a unique
// message.
// type schnorrVerifier struct {
// 	points []kyber.Point
// }

// NewVerifier returns a new verifier that can verify schnorr signatures.
// func newVerifier(points []kyber.Point) crypto.Verifier {
// 	return schnorrVerifier{points: points}
// }

// Verify implements crypto.Verifier. It returns nil if the signature matches
// the message, or an error otherwise.
// func (v schnorrVerifier) Verify(msg []byte, sig crypto.Signature) error {
// 	return xerrors.New("not implemented")
// }

type verifierFactory struct{}

// FromIterator implements crypto.VerifierFactory. It returns a verifier that
// will verify the signatures collectively signed by all the signers associated
// with the public keys.
func (v verifierFactory) FromAuthority(ca crypto.CollectiveAuthority) (crypto.Verifier, error) {
	return nil, xerrors.New("not implemented")
}

// FromArray implements crypto.VerifierFactory. It returns a verifier that will
// verify the signatures collectively signed by all the signers associated with
// the public keys.
func (v verifierFactory) FromArray(publicKeys []crypto.PublicKey) (crypto.Verifier, error) {
	return nil, xerrors.New("not implemented")
}

// Signer implements a schnorr signer
//
// - implements crypto.Signer
type Signer struct {
	keyPair *key.Pair
}

// NewSigner returns a new random schnorr signer that do NOT support
// aggregation.
func NewSigner() crypto.AggregateSigner {
	kp := key.NewKeyPair(suite)
	return Signer{
		keyPair: kp,
	}
}

// GetVerifierFactory implements crypto.Signer. It returns the verifier factory
// for schnorr signatures.
func (s Signer) GetVerifierFactory() crypto.VerifierFactory {
	return verifierFactory{}
}

// GetPublicKeyFactory implements crypto.Signer. It returns the public key
// factory for schnorr signatures.
func (s Signer) GetPublicKeyFactory() serde.Factory {
	return publicKeyFactory{}
}

// GetSignatureFactory implements crypto.Signer. It returns the signature
// factory for schnorr signatures.
func (s Signer) GetSignatureFactory() serde.Factory {
	return signatureFactory{}
}

// GetPublicKey implements crypto.Signer. It returns the public key of the
// signer that can be used to verify signatures.
func (s Signer) GetPublicKey() crypto.PublicKey {
	return PublicKey{point: s.keyPair.Public}
}

// GetPrivateKey returns the signer's private key. Needed for DKG.
func (s Signer) GetPrivateKey() kyber.Scalar {
	return s.keyPair.Private
}

// Sign implements crypto.Signer. It signs the message in parameter and returns
// the signature, or an error if it cannot sign.
func (s Signer) Sign(msg []byte) (crypto.Signature, error) {
	sig, err := schnorr.Sign(suite, s.keyPair.Private, msg)
	if err != nil {
		return nil, xerrors.Errorf("couldn't make schnorr signature: %v", err)
	}

	return signature{data: sig}, nil
}

// Aggregate implements crypto.Signer. It aggregates the signatures into a
// single one that can be verifier with the aggregated public key associated.
func (s Signer) Aggregate(signatures ...crypto.Signature) (crypto.Signature, error) {
	return nil, xerrors.New("not implemented")
}
