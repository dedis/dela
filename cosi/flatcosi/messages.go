package flatcosi

import (
	"go.dedis.ch/dela/cosi/flatcosi/json"
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

	message serde.Message
}

// VisitJSON implements serde.Message. It serializes the request in JSON format.
func (req SignatureRequest) VisitJSON(ser serde.Serializer) (interface{}, error) {
	msg, err := ser.Serialize(req.message)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize message: %v", err)
	}

	m := json.Request{
		Message: msg,
	}

	return m, nil
}

// RequestFactory is the factory for request messages.
//
// - implements serde.Factory
type RequestFactory struct {
	serde.UnimplementedFactory

	msgFactory serde.Factory
}

func newRequestFactory(f serde.Factory) RequestFactory {
	return RequestFactory{
		msgFactory: f,
	}
}

// VisitJSON implements serde.Factory. It deserializes the request message in
// JSON format.
func (f RequestFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.Request{}
	err := in.Feed(&m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize request: %v", err)
	}

	var msg serde.Message
	err = in.GetSerializer().Deserialize(m.Message, f.msgFactory, &msg)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize message: %v", err)
	}

	return SignatureRequest{message: msg}, nil
}

// SignatureResponse is the message sent by the participants.
//
// - implements serde.Message
type SignatureResponse struct {
	serde.UnimplementedMessage

	signature crypto.Signature
}

// VisitJSON implements serde.Message. It serializes the response in JSON
// format.
func (resp SignatureResponse) VisitJSON(ser serde.Serializer) (interface{}, error) {
	sig, err := ser.Serialize(resp.signature)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize signature: %v", err)
	}

	m := json.Response{
		Signature: sig,
	}

	return m, nil
}

// ResponseFactory is the factory for response messages.
//
// - implements serde.Factory
type ResponseFactory struct {
	serde.UnimplementedFactory

	sigFactory serde.Factory
}

func newResponseFactory(f serde.Factory) ResponseFactory {
	return ResponseFactory{
		sigFactory: f,
	}
}

// VisitJSON implements serde.Factory. It deserializes the response message in
// JSON format.
func (f ResponseFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.Response{}
	err := in.Feed(&m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize response: %v", err)
	}

	var sig crypto.Signature
	err = in.GetSerializer().Deserialize(m.Signature, f.sigFactory, &sig)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize signature: %v", err)
	}

	return SignatureResponse{signature: sig}, nil
}
