package cosi

import (
	"go.dedis.ch/dela/cosi/json"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// SignatureRequest is the message sent to require a signature from the other
// participants.
//
// - implements serde.Message
type SignatureRequest struct {
	serde.UnimplementedMessage

	Value serde.Message
}

// VisitJSON implements serde.Message. It serializes the request in JSON format.
func (req SignatureRequest) VisitJSON(ser serde.Serializer) (interface{}, error) {
	msg, err := ser.Serialize(req.Value)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize message: %v", err)
	}

	m := json.Request{
		Value: msg,
	}

	return json.Message{Request: &m}, nil
}

// SignatureResponse is the message sent by the participants.
//
// - implements serde.Message
type SignatureResponse struct {
	serde.UnimplementedMessage

	Signature crypto.Signature
}

// VisitJSON implements serde.Message. It serializes the response in JSON
// format.
func (resp SignatureResponse) VisitJSON(ser serde.Serializer) (interface{}, error) {
	sig, err := ser.Serialize(resp.Signature)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize signature: %v", err)
	}

	m := json.Response{
		Signature: sig,
	}

	return json.Message{Response: &m}, nil
}

// MessageFactory is the message factory for the flat collective signing RPC.
//
// - implements serde.Factory
type MessageFactory struct {
	serde.UnimplementedFactory

	msgFactory serde.Factory
	sigFactory serde.Factory
}

// NewMessageFactory returns a new message factory that uses the message and
// signature factories.
func NewMessageFactory(msg, sig serde.Factory) serde.Factory {
	return MessageFactory{
		msgFactory: msg,
		sigFactory: sig,
	}
}

// VisitJSON implements serde.Message. It deserializes the request or response
// message in JSON format.
func (f MessageFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.Message{}
	err := in.Feed(&m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize message: %v", err)
	}

	if m.Request != nil {
		var value serde.Message
		err = in.GetSerializer().Deserialize(m.Request.Value, f.msgFactory, &value)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize value: %v", err)
		}

		return SignatureRequest{Value: value}, nil
	}

	if m.Response != nil {
		var sig crypto.Signature
		err = in.GetSerializer().Deserialize(m.Response.Signature, f.sigFactory, &sig)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize signature: %v", err)
		}

		return SignatureResponse{Signature: sig}, nil
	}

	return nil, xerrors.New("message is empty")
}
