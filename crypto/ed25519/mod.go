package ed25519

import (
	"bytes"
	"fmt"

	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
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

	pubkeyFormats = registry.NewSimpleRegistry()

	sigFormats = registry.NewSimpleRegistry()
)

// RegisterPublicKeyFormat register the engine for the provided format.
func RegisterPublicKeyFormat(format serde.Format, engine serde.FormatEngine) {
	pubkeyFormats.Register(format, engine)
}

// RegisterSignatureFormat register the engine for the provided format.
func RegisterSignatureFormat(format serde.Format, engine serde.FormatEngine) {
	sigFormats.Register(format, engine)
}

// PublicKey can be provided to verify a schnorr signature.
type PublicKey struct {
	point kyber.Point
}

// NewPublicKey returns a new public key from the data.
func NewPublicKey(data []byte) (PublicKey, error) {
	point := suite.Point()
	err := point.UnmarshalBinary(data)
	if err != nil {
		return PublicKey{}, xerrors.Errorf("couldn't unmarshal point: %v", err)
	}

	pk := PublicKey{
		point: point,
	}

	return pk, nil
}

// MarshalBinary implements encoding.BinaryMarshaler. It produces a slice of
// bytes representing the public key.
func (pk PublicKey) MarshalBinary() ([]byte, error) {
	return pk.point.MarshalBinary()
}

// Serialize implements serde.Message. It returns the serialized data of the
// public key.
func (pk PublicKey) Serialize(ctx serde.Context) ([]byte, error) {
	format := pubkeyFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, pk)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode public key: %v", err)
	}

	return data, nil
}

// Verify implements crypto.PublicKey. It returns nil if the signature matches
// the message with this public key.
func (pk PublicKey) Verify(msg []byte, sig crypto.Signature) error {
	signature, ok := sig.(Signature)
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

// Signature is a proof of the integrity of a single message associated with a
// unique public key.
type Signature struct {
	data []byte
}

// NewSignature returns a new signature from the data.
func NewSignature(data []byte) Signature {
	return Signature{
		data: data,
	}
}

// MarshalBinary implements encoding.BinaryMarshaler. It returns a slice of
// bytes representing the signature.
func (sig Signature) MarshalBinary() ([]byte, error) {
	return sig.data, nil
}

// Serialize implements serde.Message. It returns the serialized data of the
// signature.
func (sig Signature) Serialize(ctx serde.Context) ([]byte, error) {
	format := sigFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, sig)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode signature: %v", err)
	}

	return data, nil
}

// Equal implements crypto.PublicKey.
func (sig Signature) Equal(other crypto.Signature) bool {
	otherSig, ok := other.(Signature)
	if !ok {
		return false
	}

	return bytes.Equal(sig.data, otherSig.data)
}

// publicKeyFactory creates schnorr compatible public key from protobuf
// messages.
//
// - implements crypto.PublicKeyFactory
// - implements serde.Factory
type publicKeyFactory struct{}

// NewPublicKeyFactory returns a new instance of the factory.
func NewPublicKeyFactory() serde.Factory {
	return publicKeyFactory{}
}

// Deserialize implements serde.Factory. It returns the public key deserialized
// if appropriate, otherwise an error.
func (f publicKeyFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	return f.PublicKeyOf(ctx, data)
}

// PublicKeyOf implements crypto.PublicKeyFactory. It returns the public key
// deserialized if appropriate, otherwise an error.
func (f publicKeyFactory) PublicKeyOf(ctx serde.Context, data []byte) (crypto.PublicKey, error) {
	format := pubkeyFormats.Get(ctx.GetFormat())

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode public key: %v", err)
	}

	pubkey, ok := msg.(PublicKey)
	if !ok {
		return nil, xerrors.Errorf("invalid public key of type '%T'", msg)
	}

	return pubkey, nil
}

// signatureFactory provides functions to create schnorr signatures from
// protobuf messages.
//
// - implements crypto.SignatureFactory
// - implements serde.Factory
type signatureFactory struct{}

// NewSignatureFactory returns a new instance of the factory.
func NewSignatureFactory() serde.Factory {
	return signatureFactory{}
}

// Deserialize implements serde.Factory.
func (f signatureFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	return f.SignatureOf(ctx, data)
}

// SignatureOf implements crypto.SignatureFactory.
func (f signatureFactory) SignatureOf(ctx serde.Context, data []byte) (crypto.Signature, error) {
	format := sigFormats.Get(ctx.GetFormat())

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode signature: %v", err)
	}

	signature, ok := msg.(Signature)
	if !ok {
		return nil, xerrors.Errorf("invalid signature of type '%T'", msg)
	}

	return signature, nil
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

// NewSigner returns a new random schnorr signer.
func NewSigner() crypto.Signer {
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
func (s Signer) GetPublicKeyFactory() crypto.PublicKeyFactory {
	return publicKeyFactory{}
}

// GetSignatureFactory implements crypto.Signer. It returns the signature
// factory for schnorr signatures.
func (s Signer) GetSignatureFactory() crypto.SignatureFactory {
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

	return Signature{data: sig}, nil
}

// Aggregate implements crypto.Signer. It aggregates the signatures into a
// single one that can be verifier with the aggregated public key associated.
func (s Signer) Aggregate(signatures ...crypto.Signature) (crypto.Signature, error) {
	return nil, xerrors.New("not implemented")
}
