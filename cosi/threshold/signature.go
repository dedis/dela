package threshold

import (
	"bytes"

	"go.dedis.ch/dela/cosi/threshold/json"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

const (
	wordlength = 8
	// shift is used to divide by 8.
	shift = 3
	// mask is used to get the remainder of a division by 8.
	mask = 0x7
)

// Signature is a threshold signature which includes an aggregated signature and
// the mask of signers from the associated collective authority.
//
// - implements crypto.Signature
type Signature struct {
	serde.UnimplementedMessage

	agg  crypto.Signature
	mask []byte
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

// VisitJSON implements serde.Message. It serializes the signature into JSON
// format.
func (s *Signature) VisitJSON(ser serde.Serializer) (interface{}, error) {
	agg, err := ser.Serialize(s.agg)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize aggregate: %v", err)
	}

	m := json.Signature{
		Mask:      s.mask,
		Aggregate: agg,
	}

	return m, nil
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

type signatureFactory struct {
	serde.UnimplementedFactory

	sigFactory serde.Factory
}

// VisitJSON implements serde.Factory. It deserializes the signature in JSON
// format.
func (f signatureFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.Signature{}
	err := in.Feed(&m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize message: %v", err)
	}

	var agg crypto.Signature
	err = in.GetSerializer().Deserialize(m.Aggregate, f.sigFactory, &agg)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize signature: %v", err)
	}

	s := &Signature{
		mask: m.Mask,
		agg:  agg,
	}

	return s, nil
}

// Verifier is a threshold verifier which can verify threshold signatures by
// aggregating public keys according to the mask.
//
// - implements crypto.Verifier
type Verifier struct {
	ca      crypto.CollectiveAuthority
	factory crypto.VerifierFactory
}

func newVerifier(ca crypto.CollectiveAuthority, f crypto.VerifierFactory) Verifier {
	return Verifier{
		ca:      ca,
		factory: f,
	}
}

// Verify implements crypto.Verifier.
func (v Verifier) Verify(msg []byte, s crypto.Signature) error {
	signature, ok := s.(*Signature)
	if !ok {
		return xerrors.Errorf("invalid signature type '%T' != '%T'", s, signature)
	}

	filter := mino.ListFilter(signature.GetIndices())
	subset := v.ca.Take(filter).(crypto.CollectiveAuthority)

	verifier, err := v.factory.FromAuthority(subset)
	if err != nil {
		return xerrors.Errorf("couldn't make verifier: %v", err)
	}

	err = verifier.Verify(msg, signature.agg)
	if err != nil {
		return xerrors.Errorf("invalid signature: %v", err)
	}

	return nil
}

type verifierFactory struct {
	factory crypto.VerifierFactory
}

func (f verifierFactory) FromAuthority(authority crypto.CollectiveAuthority) (crypto.Verifier, error) {
	return newVerifier(authority, f.factory), nil
}

func (f verifierFactory) FromArray([]crypto.PublicKey) (crypto.Verifier, error) {
	// TODO: think about this
	return nil, xerrors.New("not implemented")
}
