// Package types implements the threshold collective signature and its verifier.
//
// It wraps a signature implementation in order to extract the correct
// aggregated public key.
//
// The messages have been implemented in this isolated package so that it does
// not create cycle imports when importing the serde formats.
//
// Documentation Last Review: 05.10.2020
//
package types

import (
	"bytes"
	"fmt"
	"math/big"

	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
)

const (
	wordlength = 8
	// shift is used to divide by 8.
	shift = 3
	// mask is used to get the remainder of a division by 8.
	mask = 0x7
)

var formats = registry.NewSimpleRegistry()

// RegisterSignatureFormat saves the format to be used when
// serializing/deserializing signature messages for the given codec.
func RegisterSignatureFormat(c serde.Format, f serde.FormatEngine) {
	formats.Register(c, f)
}

// Signature is a threshold signature which includes an aggregated signature and
// the mask of signers from the associated collective authority.
//
// - implements crypto.Signature
type Signature struct {
	agg  crypto.Signature
	mask []byte
}

// NewSignature returns a new threshold signature.
func NewSignature(agg crypto.Signature, mask []byte) *Signature {
	return &Signature{
		agg:  agg,
		mask: mask,
	}
}

// GetAggregate returns the aggregate of the signature which corresponds to the
// addition of the public keys enabled in the mask.
func (s *Signature) GetAggregate() crypto.Signature {
	return s.agg
}

// GetMask returns a bit mask of which public key is enabled.
func (s *Signature) GetMask() []byte {
	return append([]byte{}, s.mask...)
}

// HasBit returns true when the bit at the given index is set to 1.
func (s *Signature) HasBit(index int) bool {
	if index < 0 {
		return false
	}

	i := index >> shift
	if i >= len(s.mask) {
		return false
	}

	return s.mask[i]&(1<<uint(index&mask)) != 0
}

// GetIndices returns the list of indices that have participated in the
// collective signature.
func (s *Signature) GetIndices() []int {
	indices := []int{}
	for i, word := range s.mask {
		for j := 0; j < wordlength; j++ {
			if word&(1<<j) != 0 {
				indices = append(indices, i*wordlength+j)
			}
		}
	}

	return indices
}

// Merge adds the signature.
func (s *Signature) Merge(signer crypto.AggregateSigner, index int, sig crypto.Signature) error {
	if s.HasBit(index) {
		return xerrors.Errorf("index %d already merged", index)
	}

	if s.agg == nil {
		s.agg = sig
		s.setBit(index)
		return nil
	}

	var err error
	s.agg, err = signer.Aggregate(s.agg, sig)
	if err != nil {
		return xerrors.Errorf("couldn't aggregate: %v", err)
	}

	s.setBit(index)

	return nil
}

func (s *Signature) setBit(index int) {
	if index < 0 {
		return
	}

	i := index >> shift
	for i >= len(s.mask) {
		s.mask = append(s.mask, 0)
	}

	s.mask[i] |= 1 << uint(index&mask)
}

// Serialize implements serde.Message. It serializes the signature into JSON
// format.
func (s *Signature) Serialize(ctx serde.Context) ([]byte, error) {
	format := formats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, s)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode signature: %v", err)
	}

	return data, nil
}

// MarshalBinary implements encoding.BinaryMarshaler.
func (s *Signature) MarshalBinary() ([]byte, error) {
	buffer, err := s.agg.MarshalBinary()
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal signature: %v", err)
	}

	buffer = append(buffer, s.mask...)

	return buffer, nil
}

// Equal implements crypto.Signature.
func (s *Signature) Equal(o crypto.Signature) bool {
	other, ok := o.(*Signature)
	return ok && other.agg.Equal(s.agg) && bytes.Equal(s.mask, other.mask)
}

// String implements fmt.Stringer. It returns a string representation of the
// signature.
func (s *Signature) String() string {
	mask := new(big.Int).SetBytes(s.mask)

	return fmt.Sprintf("thres[%b]:%s", mask, s.agg)
}

// AggKey is the key for the aggregate signature factory.
type AggKey struct{}

// SignatureFactory is the factory to deserialize collective signature.
//
// - implements crypto.SignatureFactory
// - implements serde.Factory
type SignatureFactory struct {
	aggFactory crypto.SignatureFactory
}

// NewSignatureFactory returns a new signature factory.
func NewSignatureFactory(f crypto.SignatureFactory) SignatureFactory {
	return SignatureFactory{
		aggFactory: f,
	}
}

// Deserialize implements serde.Factory. It populates the signature from the
// data if appropriate, otherwise it returns an error.
func (f SignatureFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	return f.SignatureOf(ctx, data)
}

// SignatureOf implements crypto.SignatureFactory. It populates the signature
// from the data if appropriate, otherwise it returns an error.
func (f SignatureFactory) SignatureOf(ctx serde.Context, data []byte) (crypto.Signature, error) {
	format := formats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, AggKey{}, f.aggFactory)

	m, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode signature: %v", err)
	}

	sig, ok := m.(*Signature)
	if !ok {
		return nil, xerrors.Errorf("invalid signature of type '%T'", m)
	}

	return sig, nil
}

// Verifier is a threshold verifier which can verify threshold signatures by
// aggregating public keys according to the mask.
//
// - implements crypto.Verifier
type Verifier struct {
	pubkeys []crypto.PublicKey
	factory crypto.VerifierFactory
}

func newVerifier(ca crypto.CollectiveAuthority, f crypto.VerifierFactory) Verifier {
	pubkeys := make([]crypto.PublicKey, 0, ca.Len())
	iter := ca.PublicKeyIterator()
	for iter.HasNext() {
		pubkeys = append(pubkeys, iter.GetNext())
	}

	return newVerifierArr(pubkeys, f)
}

func newVerifierArr(pubkeys []crypto.PublicKey, f crypto.VerifierFactory) Verifier {
	return Verifier{
		pubkeys: pubkeys,
		factory: f,
	}
}

// Verify implements crypto.Verifier. It returns nil if the signature matches
// the aggregate public key for the mask associated to the signature.
func (v Verifier) Verify(msg []byte, s crypto.Signature) error {
	signature, ok := s.(*Signature)
	if !ok {
		return xerrors.Errorf("invalid signature type '%T' != '%T'", s, signature)
	}

	pubkeys := make([]crypto.PublicKey, 0, len(v.pubkeys))
	for _, index := range signature.GetIndices() {
		pubkeys = append(pubkeys, v.pubkeys[index])
	}

	verifier, err := v.factory.FromArray(pubkeys)
	if err != nil {
		return xerrors.Errorf("couldn't make verifier: %v", err)
	}

	err = verifier.Verify(msg, signature.agg)
	if err != nil {
		return xerrors.Errorf("invalid signature: %v", err)
	}

	return nil
}

// VerifierFactory is a factory to create a verifier from a list of
// participants.
type verifierFactory struct {
	factory crypto.VerifierFactory
}

// NewThresholdVerifierFactory creates a new verifier factory from the
// underlying verifier factory.
func NewThresholdVerifierFactory(fac crypto.VerifierFactory) crypto.VerifierFactory {
	return verifierFactory{
		factory: fac,
	}
}

// FromAuthority implements crypto.VerifierFactory. It creates a verifier from
// the authority so that the mask's signature will pick the participants that
// have participated. The ordering of the authority must be the same.
func (f verifierFactory) FromAuthority(authority crypto.CollectiveAuthority) (crypto.Verifier, error) {
	return newVerifier(authority, f.factory), nil
}

// FromArray implements crypto.VerifierFactory. It creates a verifier from the
// list of public keys so that the mask's signature will pick the participants
// that have participated. The ordering of the keys must be the same.
func (f verifierFactory) FromArray(pubkeys []crypto.PublicKey) (crypto.Verifier, error) {
	return newVerifierArr(pubkeys, f.factory), nil
}
