package json

import (
	"go.dedis.ch/dela/crypto/common/json"
	"go.dedis.ch/dela/crypto/ed25519"
	"golang.org/x/xerrors"

	"go.dedis.ch/dela/serde"
)

func init() {
	ed25519.RegisterPublicKeyFormat(serde.FormatJSON, pubkeyFormat{})
	ed25519.RegisterSignatureFormat(serde.FormatJSON, sigFormat{})
}

type pubkeyFormat struct{}

func (f pubkeyFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	pubkey, ok := msg.(ed25519.PublicKey)
	if !ok {
		return nil, xerrors.Errorf("unsupported message of type '%T'", msg)
	}

	buffer, err := pubkey.MarshalBinary()
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal point: %v", err)
	}

	m := json.PublicKey{
		Algorithm: json.Algorithm{Name: ed25519.Algorithm},
		Data:      buffer,
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal: %v", err)
	}

	return data, nil
}

func (f pubkeyFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := json.PublicKey{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't unmarshal public key: %v", err)
	}

	pubkey, err := ed25519.NewPublicKey(m.Data)
	if err != nil {
		return nil, xerrors.Errorf("couldn't create public key: %v", err)
	}

	return pubkey, nil
}

type sigFormat struct{}

func (f sigFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	signature, ok := msg.(ed25519.Signature)
	if !ok {
		return nil, xerrors.Errorf("unsupported message of type '%T'", msg)
	}

	data, _ := signature.MarshalBinary()

	m := json.Signature{
		Algorithm: json.Algorithm{Name: ed25519.Algorithm},
		Data:      data,
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal: %v", err)
	}

	return data, nil
}

func (f sigFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := json.Signature{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't unmarshal signature: %v", err)
	}

	signature := ed25519.NewSignature(m.Data)

	return signature, nil
}
