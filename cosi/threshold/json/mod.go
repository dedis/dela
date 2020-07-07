package json

import (
	"encoding/json"

	"go.dedis.ch/dela/cosi/threshold"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	threshold.RegisterSignatureFormat(serde.FormatJSON, sigFormat{})
}

// Signature is the JSON message for the signature.
type Signature struct {
	Mask      []byte
	Aggregate json.RawMessage
}

// SigFormat is the engine to encode and decode collective signature messages in
// JSON format.
//
// - implements serde.FormatEngine
type sigFormat struct{}

// Encode implements serde.FormatEngine. It returns the serialized data of the
// signature message if appropriate, otherwise an error.
func (f sigFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	sig, ok := msg.(*threshold.Signature)
	if !ok {
		return nil, xerrors.Errorf("unsupported message of type '%T'", msg)
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
		return nil, xerrors.Errorf("couldn't marshal: %v", err)
	}

	return data, nil
}

// Decode implements serde.FormatEngine. It populates the signature of the JSON
// data if appropriate, otherwise it returns an error.
func (f sigFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := Signature{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't unmarshal message: %v", err)
	}

	factory := ctx.GetFactory(threshold.AggKey{})

	fac, ok := factory.(crypto.SignatureFactory)
	if !ok {
		return nil, xerrors.Errorf("invalid factory of type '%T'", factory)
	}

	agg, err := fac.SignatureOf(ctx, m.Aggregate)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize signature: %v", err)
	}

	s := threshold.NewSignature(agg, m.Mask)

	return s, nil
}
