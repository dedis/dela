package cosi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
)

var testCalls = &fake.Call{}

func init() {
	RegisterMessageFormat(fake.GoodFormat, fake.Format{Msg: SignatureRequest{}, Call: testCalls})
	RegisterMessageFormat(fake.BadFormat, fake.NewBadFormat())
}

func TestSignatureRequest(t *testing.T) {
	req := SignatureRequest{}

	data, err := req.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = req.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "couldn't encode request: fake error")
}

func TestSignatureResponse(t *testing.T) {
	resp := SignatureResponse{}

	data, err := resp.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = resp.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "couldn't encode response: fake error")
}

func TestMessageFactory_Deserialize(t *testing.T) {
	factory := NewMessageFactory(fake.MessageFactory{}, fake.SignatureFactory{})

	testCalls.Clear()

	msg, err := factory.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, SignatureRequest{}, msg)

	require.Equal(t, 1, testCalls.Len())
	ctx := testCalls.Get(0, 0).(serde.Context)
	require.Equal(t, fake.MessageFactory{}, ctx.GetFactory(MsgKey{}))
	require.Equal(t, fake.SignatureFactory{}, ctx.GetFactory(SigKey{}))

	_, err = factory.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, "couldn't decode message: fake error")
}
