package flatcosi

import (
	"go.dedis.ch/dela/cosi/flatcosi/json"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
)

// SignatureRequest is the message sent to require a signature from the other
// participants.
type SignatureRequest struct {
	serde.UnimplementedMessage

	message serde.Message
}

// VisitJSON implements serde.Message.
func (req SignatureRequest) VisitJSON(ser serde.Serializer) (interface{}, error) {
	msg, err := ser.Serialize(req.message)
	if err != nil {
		return nil, err
	}

	m := json.Request{
		Message: msg,
	}

	return m, nil
}

// RequestFactory is the factory for request messages.
type RequestFactory struct {
	serde.UnimplementedFactory

	msgFactory serde.Factory
}

func newRequestFactory(f serde.Factory) RequestFactory {
	return RequestFactory{
		msgFactory: f,
	}
}

// VisitJSON implements serde.Factory.
func (f RequestFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.Request{}
	err := in.Feed(&m)
	if err != nil {
		return nil, err
	}

	var msg serde.Message
	err = in.GetSerializer().Deserialize(m.Message, f.msgFactory, &msg)
	if err != nil {
		return nil, err
	}

	return SignatureRequest{message: msg}, nil
}

// SignatureResponse is the message sent by the participants.
type SignatureResponse struct {
	serde.UnimplementedMessage

	signature crypto.Signature
}

// VisitJSON implements serde.Message.
func (resp SignatureResponse) VisitJSON(ser serde.Serializer) (interface{}, error) {
	sig, err := ser.Serialize(resp.signature)
	if err != nil {
		return nil, err
	}

	m := json.Response{
		Signature: sig,
	}

	return m, nil
}

// ResponseFactory is the factory for response messages.
type ResponseFactory struct {
	serde.UnimplementedFactory

	sigFactory serde.Factory
}

func newResponseFactory(f serde.Factory) ResponseFactory {
	return ResponseFactory{
		sigFactory: f,
	}
}

// VisitJSON implements serde.Factory.
func (f ResponseFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.Response{}
	err := in.Feed(&m)
	if err != nil {
		return nil, err
	}

	var sig crypto.Signature
	err = in.GetSerializer().Deserialize(m.Signature, f.sigFactory, &sig)
	if err != nil {
		return nil, err
	}

	return SignatureResponse{signature: sig}, nil
}
