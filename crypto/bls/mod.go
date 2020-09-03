package bls

import (
	"bytes"
	"fmt"

	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/pairing"
	"go.dedis.ch/kyber/v3/sign/bls"
	"go.dedis.ch/kyber/v3/util/key"
	"golang.org/x/xerrors"
)

const (
	// Algorithm is the name of the curve used for the BLS signature.
	Algorithm = "BLS-CURVE-BN256"
)

var (
	suite = pairing.NewSuiteBn256()

	pubkeyFormats = registry.NewSimpleRegistry()
	sigFormats    = registry.NewSimpleRegistry()
)

// RegisterPublicKeyFormat registers the engine for the provided format.
func RegisterPublicKeyFormat(c serde.Format, f serde.FormatEngine) {
	pubkeyFormats.Register(c, f)
}

// RegisterSignatureFormat registers the engine for the provided format.
func RegisterSignatureFormat(c serde.Format, f serde.FormatEngine) {
	sigFormats.Register(c, f)
}

// PublicKey can be provided to verify a BLS signature.
type PublicKey struct {
	point kyber.Point
}

// NewPublicKey creates a new public key by unmarshaling the data into BN256
// point.
func NewPublicKey(data []byte) (PublicKey, error) {
	point := suite.Point()
	err := point.UnmarshalBinary(data)
	if err != nil {
		return PublicKey{}, err
	}

	return PublicKey{point: point}, nil
}

// NewPublicKeyFromPoint creates a new public key from an existing point.
func NewPublicKeyFromPoint(point kyber.Point) PublicKey {
	return PublicKey{
		point: point,
	}
}

// MarshalBinary implements encoding.BinaryMarshaler. It produces a slice of
// bytes representing the public key.
func (pk PublicKey) MarshalBinary() ([]byte, error) {
	return pk.point.MarshalBinary()
}

// Serialize implements serde.Message.
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

	err := bls.Verify(suite, pk.point, msg, signature.data)
	if err != nil {
		return xerrors.Errorf("bls verify failed: %v", err)
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

	return []byte(fmt.Sprintf("bls:%x", buffer)), nil
}

// String implements fmt.String. It returns a string representation of the
// point.
func (pk PublicKey) String() string {
	buffer, err := pk.MarshalText()
	if err != nil {
		return "bls:malformed_point"
	}

	// Output only the prefix and 16 characters of the buffer in hexadecimal.
	return string(buffer)[:4+16]
}

// Signature is a proof of the integrity of a single message associated with a
// unique public key.
type Signature struct {
	data []byte
}

// NewSignature creates a new signature from the provided data.
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

// Serialize implements serde.Message.
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

func (sig Signature) String() string {
	return fmt.Sprintf("bls:%x", sig.data)
}

// publicKeyFactory creates BLS compatible public key from protobuf messages.
//
// - serde.Factory
// - crypto.PublicKeyFactory
type publicKeyFactory struct{}

// NewPublicKeyFactory returns a new instance of the factory.
func NewPublicKeyFactory() crypto.PublicKeyFactory {
	return publicKeyFactory{}
}

// Deserialize implements serde.Factory. It looks up the format and returns the
// deserialized public key.
func (f publicKeyFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := pubkeyFormats.Get(ctx.GetFormat())

	m, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode public key: %v", err)
	}

	return m, nil
}

// PublicKeyOf implements crypto.PublicKeyFactory. It returns the public key
// deserialized from the data.
func (f publicKeyFactory) PublicKeyOf(ctx serde.Context, data []byte) (crypto.PublicKey, error) {
	m, err := f.Deserialize(ctx, data)
	if err != nil {
		return nil, err
	}

	pubkey, ok := m.(crypto.PublicKey)
	if !ok {
		return nil, xerrors.Errorf("invalid public key of type '%T'", m)
	}

	return pubkey, nil
}

func (f publicKeyFactory) FromBytes(data []byte) (crypto.PublicKey, error) {
	pubkey, err := NewPublicKey(data)
	if err != nil {
		return nil, err
	}

	return pubkey, nil
}

// signatureFactory provides functions to create BLS signatures from protobuf
// messages.
type signatureFactory struct{}

// NewSignatureFactory returns a new instance of the factory.
func NewSignatureFactory() crypto.SignatureFactory {
	return signatureFactory{}
}

// Deserialize implements serde.Factory. It looks up the format and returns the
// message of the data if appropriate, otherwise an error.
func (f signatureFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := sigFormats.Get(ctx.GetFormat())

	m, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode signature: %v", err)
	}

	return m, nil
}

// SignatureOf implements crypto.SignatureFactory. It populates the signature
// with the data if appropriate, otherwise it returns an error.
func (f signatureFactory) SignatureOf(ctx serde.Context, data []byte) (crypto.Signature, error) {
	m, err := f.Deserialize(ctx, data)
	if err != nil {
		return nil, err
	}

	sig, ok := m.(Signature)
	if !ok {
		return nil, xerrors.Errorf("invalid signature of type '%T'", m)
	}

	return sig, nil
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

	err := bls.Verify(suite, aggKey, msg, sig.(Signature).data)
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
		pk, ok := next.(PublicKey)
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
		pk, ok := pubkey.(PublicKey)
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

// NewSigner generates and returns a new random signer.
func NewSigner() crypto.AggregateSigner {
	return Generate().(crypto.AggregateSigner)
}

// Generate returns a new random BLS signer that supports aggregation.
func Generate() crypto.Signer {
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
func (s signer) GetPublicKeyFactory() crypto.PublicKeyFactory {
	return publicKeyFactory{}
}

// GetSignatureFactory implements crypto.Signer. It returns the signature
// factory for BLS signatures.
func (s signer) GetSignatureFactory() crypto.SignatureFactory {
	return signatureFactory{}
}

// GetPublicKey implements crypto.Signer. It returns the public key of the
// signer that can be used to verify signatures.
func (s signer) GetPublicKey() crypto.PublicKey {
	return PublicKey{point: s.keyPair.Public}
}

// Sign implements crypto.Signer. It signs the message in parameter and returns
// the signature, or an error if it cannot sign.
func (s signer) Sign(msg []byte) (crypto.Signature, error) {
	sig, err := bls.Sign(suite, s.keyPair.Private, msg)
	if err != nil {
		return nil, xerrors.Errorf("couldn't make bls signature: %v", err)
	}

	return Signature{data: sig}, nil
}

// Aggregate implements crypto.Signer. It aggregates the signatures into a
// single one that can be verifier with the aggregated public key associated.
func (s signer) Aggregate(signatures ...crypto.Signature) (crypto.Signature, error) {
	buffers := make([][]byte, len(signatures))
	for i, sig := range signatures {
		buffers[i] = sig.(Signature).data
	}

	agg, err := bls.AggregateSignatures(suite, buffers...)
	if err != nil {
		return nil, xerrors.Errorf("couldn't aggregate: %v", err)
	}

	return Signature{data: agg}, nil
}
