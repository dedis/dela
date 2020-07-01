package json

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/dkg/pedersen/types"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/suites"
	"golang.org/x/xerrors"
)

var suite = suites.MustFind("Ed25519")

func TestMessageFormat_Start_Encode(t *testing.T) {
	start := types.NewStart(1, nil, nil)

	format := newMsgFormat()
	ctx := serde.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, start)
	require.NoError(t, err)
	expected := `{"Start":{"Threshold":1,"Addresses":[],"PublicKeys":[]}}`
	require.Equal(t, expected, string(data))

	start = types.NewStart(0, []mino.Address{fake.NewBadAddress()}, nil)
	_, err = format.Encode(ctx, start)
	require.EqualError(t, err, "couldn't marshal address: fake error")

	start = types.NewStart(0, nil, []kyber.Point{badPoint{}})
	_, err = format.Encode(ctx, start)
	require.EqualError(t, err, "couldn't marshal public key: oops")
}

func TestMessageFormat_StartDone_Encode(t *testing.T) {
	done := types.NewStartDone(suite.Point())

	format := newMsgFormat()
	ctx := serde.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, done)
	require.NoError(t, err)
	require.Regexp(t, `{(("StartDone":{"PublicKey":"[^"]+"}|"\w+":null),?)+}`, string(data))

	done = types.NewStartDone(badPoint{})
	_, err = format.Encode(ctx, done)
	require.EqualError(t, err, "couldn't marshal public key: oops")
}

func TestMessageFormat_DecryptRequest_Encode(t *testing.T) {
	req := types.NewDecryptRequest(suite.Point(), suite.Point())

	format := newMsgFormat()
	ctx := serde.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, req)
	require.NoError(t, err)
	require.Regexp(t, `{(("DecryptRequest":{"K":"[^"]+","C":"[^"]+"}|"\w+":null),?)+}`, string(data))

	req.K = badPoint{}
	_, err = format.Encode(ctx, req)
	require.EqualError(t, err, "couldn't marshal K: oops")

	req.K = suite.Point()
	req.C = badPoint{}
	_, err = format.Encode(ctx, req)
	require.EqualError(t, err, "couldn't marshal C: oops")
}

func TestMessageFormat_DecryptReply_Encode(t *testing.T) {
	resp := types.NewDecryptReply(5, suite.Point())

	format := newMsgFormat()
	ctx := serde.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, resp)
	require.NoError(t, err)
	require.Regexp(t, `{(("DecryptReply":{"V":"[^"]+","I":5}|"\w+":null),?)+}`, string(data))

	resp.V = badPoint{}
	_, err = format.Encode(ctx, resp)
	require.EqualError(t, err, "couldn't marshal V: oops")
}

func TestMessageFormat_Decode(t *testing.T) {
	format := newMsgFormat()
	ctx := serde.NewContext(fake.ContextEngine{})
	ctx = serde.WithFactory(ctx, types.AddrKey{}, fake.AddressFactory{})

	start, err := format.Decode(ctx, []byte(`{"Start":{}}`))
	require.NoError(t, err)
	require.Equal(t, types.NewStart(0, []mino.Address{}, []kyber.Point{}), start)

	_, err = format.Decode(ctx, []byte(`{"Start":{"PublicKeys":[[]]}}`))
	require.EqualError(t, err,
		"couldn't unmarshal public key: invalid Ed25519 curve point")

	deal, err := format.Decode(ctx, []byte(`{"Deal":{}}`))
	require.NoError(t, err)
	require.Equal(t, types.NewDeal(0, nil, types.EncryptedDeal{}), deal)

	resp, err := format.Decode(ctx, []byte(`{"Response":{}}`))
	require.NoError(t, err)
	require.Equal(t, types.NewResponse(0, types.DealerResponse{}), resp)

	data := []byte(fmt.Sprintf(`{"StartDone":{"PublicKey":"%s"}}`, testPoint))
	done, err := format.Decode(ctx, data)
	require.NoError(t, err)
	require.IsType(t, types.StartDone{}, done)

	data = []byte(`{"StartDone":{"PublicKey":[]}}`)
	_, err = format.Decode(ctx, data)
	require.EqualError(t, err,
		"couldn't unmarshal public key: invalid Ed25519 curve point")

	data = []byte(fmt.Sprintf(`{"DecryptRequest":{"K":"%s","C":"%s"}}`, testPoint, testPoint))
	req, err := format.Decode(ctx, data)
	require.NoError(t, err)
	require.IsType(t, types.DecryptRequest{}, req)

	data = []byte(fmt.Sprintf(`{"DecryptRequest":{"K":[],"C":"%s"}}`, testPoint))
	_, err = format.Decode(ctx, data)
	require.EqualError(t, err,
		"couldn't unmarshal K: invalid Ed25519 curve point")

	data = []byte(fmt.Sprintf(`{"DecryptRequest":{"K":"%s","C":[]}}`, testPoint))
	_, err = format.Decode(ctx, data)
	require.EqualError(t, err,
		"couldn't unmarshal C: invalid Ed25519 curve point")

	data = []byte(fmt.Sprintf(`{"DecryptReply":{"I":4,"V":"%s"}}`, testPoint))
	resp, err = format.Decode(ctx, data)
	require.NoError(t, err)
	require.IsType(t, types.DecryptReply{}, resp)

	data = []byte(`{"DecryptReply":{"V":[]}}`)
	_, err = format.Decode(ctx, data)
	require.EqualError(t, err,
		"couldn't unmarshal V: invalid Ed25519 curve point")

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, "couldn't deserialize message: fake error")

	_, err = format.Decode(ctx, []byte(`{}`))
	require.EqualError(t, err, "message is empty")
}

// -----------------------------------------------------------------------------
// Utility functions

const testPoint = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

type badPoint struct {
	kyber.Point
}

func (p badPoint) MarshalBinary() ([]byte, error) {
	return nil, xerrors.New("oops")
}
