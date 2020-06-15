package pedersen

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde/json"
	"go.dedis.ch/kyber/v3"
	"golang.org/x/xerrors"
)

func TestStart_VisitJSON(t *testing.T) {
	start := Start{}

	ser := json.NewSerializer()

	data, err := ser.Serialize(start)
	require.NoError(t, err)
	regexp := `{(("Start":{"Threshold":0,"Addresses":\[\],"PublicKeys":\[\]}|"\w+":null),?)+}`
	require.Regexp(t, regexp, string(data))

	start.addresses = []mino.Address{fake.NewBadAddress()}
	_, err = start.VisitJSON(ser)
	require.EqualError(t, err, "couldn't marshal address: fake error")

	start.addresses = nil
	start.pubkeys = []kyber.Point{badPoint{}}
	_, err = start.VisitJSON(ser)
	require.EqualError(t, err, "couldn't marshal public key: oops")
}

func TestStartDone_VisitJSON(t *testing.T) {
	done := StartDone{
		pubkey: suite.Point(),
	}

	ser := json.NewSerializer()

	data, err := ser.Serialize(done)
	require.NoError(t, err)
	require.Regexp(t, `{(("StartDone":{"PublicKey":"[^"]+"}|"\w+":null),?)+}`, string(data))

	done.pubkey = badPoint{}
	_, err = done.VisitJSON(ser)
	require.EqualError(t, err, "couldn't marshal public key: oops")
}

func TestDecryptRequest_VisitJSON(t *testing.T) {
	req := DecryptRequest{
		K: suite.Point(),
		C: suite.Point(),
	}

	ser := json.NewSerializer()

	data, err := ser.Serialize(req)
	require.NoError(t, err)
	require.Regexp(t, `{(("DecryptRequest":{"K":"[^"]+","C":"[^"]+"}|"\w+":null),?)+}`, string(data))

	req.K = badPoint{}
	_, err = req.VisitJSON(ser)
	require.EqualError(t, err, "couldn't marshal K: oops")

	req.K = suite.Point()
	req.C = badPoint{}
	_, err = req.VisitJSON(ser)
	require.EqualError(t, err, "couldn't marshal C: oops")
}

func TestDecryptReply_VisitJSON(t *testing.T) {
	resp := DecryptReply{
		I: 5,
		V: suite.Point(),
	}

	ser := json.NewSerializer()

	data, err := ser.Serialize(resp)
	require.NoError(t, err)
	require.Regexp(t, `{(("DecryptReply":{"V":"[^"]+","I":5}|"\w+":null),?)+}`, string(data))

	resp.V = badPoint{}
	_, err = resp.VisitJSON(ser)
	require.EqualError(t, err, "couldn't marshal V: oops")
}

func TestMessageFactory_VisitJSON(t *testing.T) {
	factory := NewMessageFactory(suite, fake.AddressFactory{})

	ser := json.NewSerializer()

	var start Start
	err := ser.Deserialize([]byte(`{"Start":{}}`), factory, &start)
	require.NoError(t, err)

	err = ser.Deserialize([]byte(`{"Start":{"PublicKeys":[[]]}}`), factory, &start)
	require.EqualError(t, xerrors.Unwrap(err),
		"couldn't unmarshal public key: invalid Ed25519 curve point")

	var deal Deal
	err = ser.Deserialize([]byte(`{"Deal":{}}`), factory, &deal)
	require.NoError(t, err)

	var response Response
	err = ser.Deserialize([]byte(`{"Response":{}}`), factory, &response)
	require.NoError(t, err)

	var done StartDone
	data := []byte(fmt.Sprintf(`{"StartDone":{"PublicKey":"%s"}}`, testPoint))
	err = ser.Deserialize(data, factory, &done)
	require.NoError(t, err)

	data = []byte(`{"StartDone":{"PublicKey":[]}}`)
	err = ser.Deserialize(data, factory, &done)
	require.EqualError(t, xerrors.Unwrap(err),
		"couldn't unmarshal public key: invalid Ed25519 curve point")

	var req DecryptRequest
	data = []byte(fmt.Sprintf(`{"DecryptRequest":{"K":"%s","C":"%s"}}`, testPoint, testPoint))
	err = ser.Deserialize(data, factory, &req)
	require.NoError(t, err)

	data = []byte(fmt.Sprintf(`{"DecryptRequest":{"K":[],"C":"%s"}}`, testPoint))
	err = ser.Deserialize(data, factory, &req)
	require.EqualError(t, xerrors.Unwrap(err),
		"couldn't unmarshal K: invalid Ed25519 curve point")

	data = []byte(fmt.Sprintf(`{"DecryptRequest":{"K":"%s","C":[]}}`, testPoint))
	err = ser.Deserialize(data, factory, &req)
	require.EqualError(t, xerrors.Unwrap(err),
		"couldn't unmarshal C: invalid Ed25519 curve point")

	var resp DecryptReply
	data = []byte(fmt.Sprintf(`{"DecryptReply":{"I":4,"V":"%s"}}`, testPoint))
	err = ser.Deserialize(data, factory, &resp)
	require.NoError(t, err)

	data = []byte(`{"DecryptReply":{"V":[]}}`)
	err = ser.Deserialize(data, factory, &resp)
	require.EqualError(t, xerrors.Unwrap(err),
		"couldn't unmarshal V: invalid Ed25519 curve point")

	_, err = factory.VisitJSON(fake.NewBadFactoryInput())
	require.EqualError(t, err, "couldn't deserialize message: fake error")

	err = ser.Deserialize([]byte(`{}`), factory, nil)
	require.EqualError(t, xerrors.Unwrap(err), "message is empty")
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
