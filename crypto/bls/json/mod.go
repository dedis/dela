package json

import (
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/crypto/common/json"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	bls.RegisterPublicKeyFormat(serde.FormatJSON, pubkeyFormat{})
	bls.RegisterSignatureFormat(serde.FormatJSON, sigFormat{})
}

// PubkeyFormat is the engine to encode and decode BLS-BN256 public keys in JSON
// format.
//
// - implements serde.FormatEngine
type pubkeyFormat struct{}

// Encode implements serde.FormatEngine. It serialized the public key message in
// JSON if appropriate, otherwise it returns an error.
func (f pubkeyFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	pubkey, ok := msg.(bls.PublicKey)
	if !ok {
		return nil, xerrors.Errorf("unsupported message of type '%T'", msg)
	}

	buffer, err := pubkey.MarshalBinary()
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal point: %v", err)
	}

	m := json.PublicKey{
		Algorithm: json.Algorithm{Name: bls.Algorithm},
		Data:      buffer,
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal: %v", err)
	}

	return data, nil
}

// Decode implements serde.FormatEngine. It populates the public key with JSON
// data if appropriate, otherwise it returns an error.
func (f pubkeyFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := json.PublicKey{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize data: %v", err)
	}

	pubkey, err := bls.NewPublicKey(m.Data)
	if err != nil {
		return nil, xerrors.Errorf("couldn't unmarshal point: %v", err)
	}

	return pubkey, nil
}

// SigFormat is the engine to encode and decode signature messages in JSON
// format.
//
// - implements serde.FormatEngine
type sigFormat struct{}

// Encode implements serde.FormatEngine. It returns the serialized data of the
// signature message if appropriate, otherwise an error.
func (f sigFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	sig, ok := msg.(bls.Signature)
	if !ok {
		return nil, xerrors.Errorf("unsupported message of type '%T'", msg)
	}

	buffer, err := sig.MarshalBinary()
	assert(err)

	m := json.Signature{
		Algorithm: json.Algorithm{Name: bls.Algorithm},
		Data:      buffer,
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal: %v", err)
	}

	return data, nil
}

// Decode implements serde.FormatEngine. It populates the signature with the
// JSON data if appropriate, otherwise it returns an error.
func (f sigFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := json.Signature{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize data: %v", err)
	}

	return bls.NewSignature(m.Data), nil
}

// Current implementation cannot return an error but it might change in the
// future therefore an assertion is made to detect if it changes.
func assert(err error) {
	if err != nil {
		panic("Implementation of the BLS signature is expected " +
			"to return a nil when marshaling but an error has been found: " + err.Error())
	}
}
