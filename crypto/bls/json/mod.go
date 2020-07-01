package json

import (
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/crypto/common/json"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	bls.RegisterPublicKey(serde.FormatJSON, pubkeyFormat{})
	bls.RegisterSignature(serde.FormatJSON, sigFormat{})
}

type pubkeyFormat struct{}

func (f pubkeyFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	pubkey, ok := msg.(bls.PublicKey)
	if !ok {
		return nil, xerrors.New("invalid bls public key")
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
		return nil, err
	}

	return data, nil
}

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

type sigFormat struct{}

func (f sigFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	sig, ok := msg.(bls.Signature)
	if !ok {
		return nil, xerrors.New("invalid signature")
	}

	// The BLS signature cannot return an error so it is ignored.
	// TODO: runtime assertion
	buffer, _ := sig.MarshalBinary()

	m := json.Signature{
		Algorithm: json.Algorithm{Name: bls.Algorithm},
		Data:      buffer,
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (f sigFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := json.Signature{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize data: %v", err)
	}

	return bls.NewSignature(m.Data), nil
}
