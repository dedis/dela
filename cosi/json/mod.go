package json

import (
	"encoding/json"

	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serdeng"
	"golang.org/x/xerrors"
)

func init() {
	cosi.Register(serdeng.CodecJSON, format{})
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

type format struct{}

func (f format) Encode(ctx serdeng.Context, msg serdeng.Message) ([]byte, error) {
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

	return ctx.Marshal(m)
}

func (f format) Decode(ctx serdeng.Context, data []byte) (serdeng.Message, error) {
	m := Message{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}

	if m.Request != nil {
		factory := ctx.GetFactory(cosi.MsgKey{})
		if factory == nil {
			return nil, xerrors.Errorf("factory is nil")
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

	return cosi.SignatureResponse{}, nil
}
