package flatcosi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde/json"
)

func TestSignatureRequest_VisitJSON(t *testing.T) {
	req := SignatureRequest{
		message: fake.Message{},
	}

	ser := json.NewSerializer()

	data, err := ser.Serialize(req)
	require.NoError(t, err)
	require.Equal(t, `{"Message":{}}`, string(data))

	_, err = req.VisitJSON(fake.NewBadSerializer())
	require.EqualError(t, err, "couldn't serialize message: fake error")
}

func TestRequestFactory_VisitJSON(t *testing.T) {
	factory := RequestFactory{
		msgFactory: fake.MessageFactory{},
	}

	ser := json.NewSerializer()

	var req SignatureRequest
	err := ser.Deserialize([]byte(`{}`), factory, &req)
	require.NoError(t, err)

	_, err = factory.VisitJSON(fake.NewBadFactoryInput())
	require.EqualError(t, err, "couldn't deserialize request: fake error")

	_, err = factory.VisitJSON(fake.FactoryInput{Serde: fake.NewBadSerializer()})
	require.EqualError(t, err, "couldn't deserialize message: fake error")
}

func TestSignatureResponse_VisitJSON(t *testing.T) {
	resp := SignatureResponse{
		signature: fake.Signature{},
	}

	ser := json.NewSerializer()

	data, err := ser.Serialize(resp)
	require.NoError(t, err)
	require.Equal(t, `{"Signature":{}}`, string(data))

	_, err = resp.VisitJSON(fake.NewBadSerializer())
	require.EqualError(t, err, "couldn't serialize signature: fake error")
}

func TestResponseFactory_VisitJSON(t *testing.T) {
	factory := ResponseFactory{
		sigFactory: fake.SignatureFactory{},
	}

	ser := json.NewSerializer()

	var resp SignatureResponse
	err := ser.Deserialize([]byte(`{}`), factory, &resp)
	require.NoError(t, err)

	_, err = factory.VisitJSON(fake.NewBadFactoryInput())
	require.EqualError(t, err, "couldn't deserialize response: fake error")

	_, err = factory.VisitJSON(fake.FactoryInput{Serde: fake.NewBadSerializer()})
	require.EqualError(t, err, "couldn't deserialize signature: fake error")
}
