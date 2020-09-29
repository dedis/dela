package json

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
)

func TestMsgFormat_Encode(t *testing.T) {
	req := cosi.SignatureRequest{
		Value: fake.Message{},
	}

	format := msgFormat{}
	ctx := fake.NewContextWithFormat(serde.FormatJSON)

	data, err := format.Encode(ctx, req)
	require.NoError(t, err)
	require.Equal(t, `{"Request":{"Value":{}}}`, string(data))

	req.Value = fake.NewBadPublicKey()
	_, err = format.Encode(ctx, req)
	require.EqualError(t, err, "couldn't serialize message: fake error")

	req.Value = fake.PublicKey{}
	_, err = format.Encode(fake.NewBadContext(), req)
	require.EqualError(t, err, "couldn't marshal: fake error")
}

func TestMsgFormat_SignatureResponse_Encode(t *testing.T) {
	resp := cosi.SignatureResponse{
		Signature: fake.Signature{},
	}

	format := msgFormat{}
	ctx := fake.NewContextWithFormat(serde.FormatJSON)

	data, err := format.Encode(ctx, resp)
	require.NoError(t, err)
	require.Equal(t, `{"Response":{"Signature":{}}}`, string(data))

	resp.Signature = fake.NewBadSignature()
	_, err = format.Encode(ctx, resp)
	require.EqualError(t, err, "couldn't serialize signature: fake error")
}

func TestMsgFormat_Decode(t *testing.T) {
	format := msgFormat{}
	ctx := fake.NewContextWithFormat(serde.FormatJSON)
	ctx = serde.WithFactory(ctx, cosi.MsgKey{}, fake.MessageFactory{})
	ctx = serde.WithFactory(ctx, cosi.SigKey{}, fake.SignatureFactory{})

	msg, err := format.Decode(ctx, []byte(`{"Request":{}}`))
	require.NoError(t, err)
	require.Equal(t, cosi.SignatureRequest{Value: fake.Message{}}, msg)

	badCtx := serde.WithFactory(ctx, cosi.MsgKey{}, fake.NewBadMessageFactory())
	_, err = format.Decode(badCtx, []byte(`{"Request":{}}`))
	require.EqualError(t, err, "couldn't deserialize value: fake error")

	badCtx = serde.WithFactory(ctx, cosi.MsgKey{}, nil)
	_, err = format.Decode(badCtx, []byte(`{"Request":{}}`))
	require.EqualError(t, err, "factory is nil")

	msg, err = format.Decode(ctx, []byte(`{"Response":{}}`))
	require.NoError(t, err)
	require.Equal(t, cosi.SignatureResponse{Signature: fake.Signature{}}, msg)

	badCtx = serde.WithFactory(ctx, cosi.SigKey{}, fake.NewBadSignatureFactory())
	_, err = format.Decode(badCtx, []byte(`{"Response":{}}`))
	require.EqualError(t, err, "couldn't deserialize signature: fake error")

	badCtx = serde.WithFactory(ctx, cosi.SigKey{}, nil)
	_, err = format.Decode(badCtx, []byte(`{"Response":{}}`))
	require.EqualError(t, err, "invalid factory of type '<nil>'")

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, "couldn't unmarshal message: fake error")

	_, err = format.Decode(ctx, []byte(`{}`))
	require.EqualError(t, err, "message is empty")
}
