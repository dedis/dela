package cosi

import (
	"testing"

	"github.com/stretchr/testify/require"
	types "go.dedis.ch/dela/cosi/json"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde/json"
)

func TestSignatureRequest_VisitJSON(t *testing.T) {
	req := SignatureRequest{
		Value: fake.Message{},
	}

	ser := json.NewSerializer()

	data, err := ser.Serialize(req)
	require.NoError(t, err)
	require.Equal(t, `{"Request":{"Value":{}}}`, string(data))

	_, err = req.VisitJSON(fake.NewBadSerializer())
	require.EqualError(t, err, "couldn't serialize message: fake error")
}

func TestSignatureResponse_VisitJSON(t *testing.T) {
	resp := SignatureResponse{
		Signature: fake.Signature{},
	}

	ser := json.NewSerializer()

	data, err := ser.Serialize(resp)
	require.NoError(t, err)
	require.Equal(t, `{"Response":{"Signature":{}}}`, string(data))

	_, err = resp.VisitJSON(fake.NewBadSerializer())
	require.EqualError(t, err, "couldn't serialize signature: fake error")
}

func TestMessageFactory_VisitJSON(t *testing.T) {
	factory := NewMessageFactory(fake.MessageFactory{}, fake.SignatureFactory{})

	ser := json.NewSerializer()

	_, err := factory.VisitJSON(fake.NewBadFactoryInput())
	require.EqualError(t, err, "couldn't deserialize message: fake error")

	// Request message.
	var req SignatureRequest
	err = ser.Deserialize([]byte(`{"Request":{}}`), factory, &req)
	require.NoError(t, err)

	input := fake.FactoryInput{
		Serde:   fake.NewBadSerializer(),
		Message: types.Message{Request: &types.Request{}},
	}
	_, err = factory.VisitJSON(input)
	require.EqualError(t, err, "couldn't deserialize value: fake error")

	// Response message.
	var resp SignatureResponse
	err = ser.Deserialize([]byte(`{"Response":{}}`), factory, &resp)
	require.NoError(t, err)

	input.Message = types.Message{Response: &types.Response{}}
	_, err = factory.VisitJSON(input)
	require.EqualError(t, err, "couldn't deserialize signature: fake error")
}
