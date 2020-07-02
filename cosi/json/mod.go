package json

import (
	"encoding/json"

	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	cosi.RegisterMessageFormat(serde.FormatJSON, msgFormat{})
}

// Request is the JSON message sent to request signature.
type Request struct {
	Value json.RawMessage
}

// Response is the JSON message sent to respond with a signature.
type Response struct {
	Signature json.RawMessage
}

// Message is a JSON container to differentiate the different messages of flat
// collective signing.
type Message struct {
	Request  *Request  `json:",omitempty"`
	Response *Response `json:",omitempty"`
}

// MsgFormat is the engine to encode and decode collective signing messages in
// JSON format.
//
// - implements serde.FormatEngine
type msgFormat struct{}

// Decode implements serde.FormatEngine. It returns the serialized data of a
// message in JSON format.
func (f msgFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	m := Message{}

	switch message := msg.(type) {
	case cosi.SignatureRequest:
		value, err := message.Value.Serialize(ctx)
		if err != nil {
			return nil, xerrors.Errorf("couldn't serialize message: %v", err)
		}

		m.Request = &Request{
			Value: value,
		}
	case cosi.SignatureResponse:
		sig, err := message.Signature.Serialize(ctx)
		if err != nil {
			return nil, xerrors.Errorf("couldn't serialize signature: %v", err)
		}

		m.Response = &Response{
			Signature: sig,
		}
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal: %v", err)
	}

	return data, nil
}

// Decode implements serde.FormatEngine. It populates the message with the JSON
// data if appropriate, otherwise it returns an error.
func (f msgFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := Message{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't unmarshal message: %v", err)
	}

	if m.Request != nil {
		factory := ctx.GetFactory(cosi.MsgKey{})
		if factory == nil {
			return nil, xerrors.New("factory is nil")
		}

		value, err := factory.Deserialize(ctx, m.Request.Value)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize value: %v", err)
		}

		return cosi.SignatureRequest{Value: value}, nil
	}

	if m.Response != nil {
		factory := ctx.GetFactory(cosi.SigKey{})

		fac, ok := factory.(crypto.SignatureFactory)
		if !ok {
			return nil, xerrors.Errorf("invalid factory of type '%T'", factory)
		}

		sig, err := fac.SignatureOf(ctx, m.Response.Signature)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize signature: %v", err)
		}

		return cosi.SignatureResponse{Signature: sig}, nil
	}

	return nil, xerrors.Errorf("message is empty")
}
