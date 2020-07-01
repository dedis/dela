package json

import (
	"encoding/json"

	"go.dedis.ch/dela/cosi/threshold"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	threshold.Register(serde.CodecJSON, format{})
}

// Signature is the JSON message for the signature.
type Signature struct {
	Mask      []byte
	Aggregate json.RawMessage
}

type format struct{}

func (f format) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	sig, ok := msg.(*threshold.Signature)
	if !ok {
		return nil, xerrors.Errorf("invalid message type")
	}

	agg, err := sig.GetAggregate().Serialize(ctx)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize aggregate: %v", err)
	}

	m := Signature{
		Mask:      sig.GetMask(),
		Aggregate: json.RawMessage(agg),
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal message: %v", err)
	}

	return data, nil
}

func (f format) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := Signature{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't unmarshal message: %v", err)
	}

	factory, ok := ctx.GetFactory(threshold.AggKey{}).(crypto.SignatureFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid factory")
	}

	agg, err := factory.SignatureOf(ctx, m.Aggregate)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize signature: %v", err)
	}

	s := threshold.NewSignature(agg, m.Mask)

	return s, nil
}
