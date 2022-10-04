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
)

// suite is the Kyber suite for Pedersen.
var suite = suites.MustFind("Ed25519")

func TestMessageFormat_Start_Encode(t *testing.T) {
	start := types.NewStart(1, []mino.Address{fake.NewAddress(0)}, []kyber.Point{suite.Point()})

	format := newMsgFormat()
	ctx := serde.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, start)
	require.NoError(t, err)
	regexp := `{"Start":{"Threshold":1,"Addresses":\["AAAAAA=="\],"PublicKeys":\["[^"]+"\]}}`
	require.Regexp(t, regexp, string(data))

	start = types.NewStart(0, []mino.Address{fake.NewBadAddress()}, nil)
	_, err = format.Encode(ctx, start)
	require.EqualError(t, err, fake.Err("failed to encode message: couldn't marshal address"))

	start = types.NewStart(0, nil, []kyber.Point{badPoint{}})
	_, err = format.Encode(ctx, start)
	require.EqualError(t, err, fake.Err("failed to encode message: couldn't marshal public key"))

	_, err = format.Encode(fake.NewBadContext(), types.Start{})
	require.EqualError(t, err, fake.Err("couldn't marshal"))

	_, err = format.Encode(ctx, fake.Message{})
	require.EqualError(t, err, "unsupported message of type 'fake.Message'")
}

func TestMessageFormat_StartResharing_Encode(t *testing.T) {
	start := types.NewStartResharing(1, 1, []mino.Address{fake.NewAddress(0)},
		[]mino.Address{fake.NewAddress(1)}, []kyber.Point{suite.Point()},
		[]kyber.Point{suite.Point()})

	format := newMsgFormat()
	ctx := serde.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, start)
	require.NoError(t, err)
	regexp := `{"StartResharing":{"TNew":1,"TOld":1,"AddrsNew":\["AAAAAA=="\],"AddrsOld":\["AQAAAA=="\],"PubkeysNew":\["[^"]+"\],"PubkeysOld":\["[^"]+"\]}}`
	require.Regexp(t, regexp, string(data))

	start = types.NewStartResharing(1, 1, []mino.Address{fake.NewBadAddress()}, nil, nil, nil)
	_, err = format.Encode(ctx, start)
	require.EqualError(t, err, fake.Err("failed to encode message: couldn't marshal new address"))

	start = types.NewStartResharing(1, 1, nil, []mino.Address{fake.NewBadAddress()}, nil, nil)
	_, err = format.Encode(ctx, start)
	require.EqualError(t, err, fake.Err("failed to encode message: couldn't marshal old address"))

	start = types.NewStartResharing(1, 1, nil, nil, []kyber.Point{badPoint{}}, nil)
	_, err = format.Encode(ctx, start)
	require.EqualError(t, err, fake.Err("failed to encode message: couldn't marshal new public key"))

	start = types.NewStartResharing(1, 1, nil, nil, nil, []kyber.Point{badPoint{}})
	_, err = format.Encode(ctx, start)
	require.EqualError(t, err, fake.Err("failed to encode message: couldn't marshal old public key"))
}

func TestMessageFormat_Deal_Encode(t *testing.T) {
	deal := types.NewDeal(1, []byte{1}, types.EncryptedDeal{})

	format := newMsgFormat()
	ctx := serde.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, deal)
	require.NoError(t, err)
	expected := `{"Deal":{"Index":1,"Signature":"AQ==","EncryptedDeal":{"DHKey":"","Signature":"","Nonce":"","Cipher":""}}}`
	require.Equal(t, expected, string(data))
}

func TestMessageFormat_Reshare_Encode(t *testing.T) {
	reshare := types.NewReshare(types.Deal{}, []kyber.Point{suite.Point()})
	format := newMsgFormat()
	ctx := serde.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, reshare)
	require.NoError(t, err)
	regexp := `{"Reshare":{"Deal":{"Index":0,"Signature":"","EncryptedDeal":{"DHKey":"","Signature":"","Nonce":"","Cipher":""}},"PublicCoeff":\["[^"]+"\]}}`
	require.Regexp(t, regexp, string(data))

	reshare = types.NewReshare(types.Deal{}, []kyber.Point{badPoint{}})
	_, err = format.Encode(ctx, reshare)
	require.EqualError(t, err, fake.Err("failed to encode message: couldn't marshal public coefficient"))
}

func TestMessageFormat_Response_Encode(t *testing.T) {
	resp := types.NewResponse(1, types.DealerResponse{})

	format := newMsgFormat()
	ctx := serde.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, resp)
	require.NoError(t, err)
	expected := `{"Response":{"Index":1,"Response":{"SessionID":"","Index":0,"Status":false,"Signature":""}}}`
	require.Equal(t, expected, string(data))
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
	require.EqualError(t, err, fake.Err("failed to encode message: couldn't marshal public key"))
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
	require.EqualError(t, err, fake.Err("failed to encode message: couldn't marshal K"))

	req.K = suite.Point()
	req.C = badPoint{}
	_, err = format.Encode(ctx, req)
	require.EqualError(t, err, fake.Err("failed to encode message: couldn't marshal C"))
}

func TestMessageFormat_VerifiableDecryptRequest_Encode(t *testing.T) {
	req := types.NewVerifiableDecryptRequest([]types.Ciphertext{{
		K:    suite.Point(),
		C:    suite.Point(),
		UBar: suite.Point(),
		E:    suite.Scalar(),
		F:    suite.Scalar(),
		GBar: suite.Point(),
	}})

	format := newMsgFormat()
	ctx := serde.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, req)
	require.NoError(t, err)
	regexp := `{"VerifiableDecryptRequest":{"Ciphertexts":\[{"K":"[^"]+","C":"[^"]+","UBar":"[^"]+","E":"[^"]+","F":"[^"]+","GBar":"[^"]+"}\]}}`
	require.Regexp(t, regexp, string(data))

	check := func(attr string, ct types.Ciphertext) func(t *testing.T) {
		return func(t *testing.T) {
			req := types.NewVerifiableDecryptRequest([]types.Ciphertext{ct})
			_, err = format.Encode(ctx, req)
			require.EqualError(t, err, fake.Err("failed to encode message: couldn't marshal "+attr))
		}
	}

	t.Run("K", check("K", types.Ciphertext{K: badPoint{}}))
	t.Run("C", check("C", types.Ciphertext{K: suite.Point(), C: badPoint{}}))
	t.Run("UBar", check("UBar", types.Ciphertext{K: suite.Point(), C: suite.Point(), UBar: badPoint{}}))
	t.Run("E", check("E", types.Ciphertext{K: suite.Point(), C: suite.Point(), UBar: suite.Point(), E: badScallar{}}))
	t.Run("F", check("F", types.Ciphertext{K: suite.Point(), C: suite.Point(), UBar: suite.Point(), E: suite.Scalar(), F: badScallar{}}))
	t.Run("GBar", check("GBar", types.Ciphertext{K: suite.Point(), C: suite.Point(), UBar: suite.Point(), E: suite.Scalar(), F: suite.Scalar(), GBar: badPoint{}}))
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
	require.EqualError(t, err, fake.Err("failed to encode message: couldn't marshal V"))
}

func TestMessageFormat_VerifiableDecryptReply_Encode(t *testing.T) {
	req := types.NewVerifiableDecryptReply([]types.ShareAndProof{{
		V:  suite.Point(),
		I:  int64(0),
		Ui: suite.Point(),
		Ei: suite.Scalar(),
		Fi: suite.Scalar(),
		Hi: suite.Point(),
	}})

	format := newMsgFormat()
	ctx := serde.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, req)
	require.NoError(t, err)
	regexp := `{"VerifiableDecryptReply":{"Sp":\[{"V":"[^"]+","I":0,"Ui":"[^"]+","Ei":"[^"]+","Fi":"[^"]+","Hi":"[^"]+"}\]}}`
	require.Regexp(t, regexp, string(data))

	check := func(attr string, sp types.ShareAndProof) func(t *testing.T) {
		return func(t *testing.T) {
			req := types.NewVerifiableDecryptReply([]types.ShareAndProof{sp})
			_, err = format.Encode(ctx, req)
			require.EqualError(t, err, fake.Err("failed to encode message: couldn't marshal "+attr))
		}
	}

	t.Run("V", check("V", types.ShareAndProof{V: badPoint{}}))
	t.Run("Ui", check("U_i", types.ShareAndProof{V: suite.Point(), Ui: badPoint{}}))
	t.Run("Ei", check("E_i", types.ShareAndProof{V: suite.Point(), Ui: suite.Point(), Ei: badScallar{}}))
	t.Run("Fi", check("F_i", types.ShareAndProof{V: suite.Point(), Ui: suite.Point(), Ei: suite.Scalar(), Fi: badScallar{}}))
	t.Run("Hi", check("H_i", types.ShareAndProof{V: suite.Point(), Ui: suite.Point(), Ei: suite.Scalar(), Fi: suite.Scalar(), Hi: badPoint{}}))
}

func TestMessageFormat_Decode(t *testing.T) {
	format := newMsgFormat()
	ctx := serde.NewContext(fake.ContextEngine{})
	ctx = serde.WithFactory(ctx, types.AddrKey{}, fake.AddressFactory{})

	// Decode start messages.
	expected := types.NewStart(
		5,
		[]mino.Address{fake.NewAddress(0)},
		[]kyber.Point{suite.Point()},
	)

	data, err := format.Encode(ctx, expected)
	require.NoError(t, err)

	start, err := format.Decode(ctx, data)
	require.NoError(t, err)
	require.Equal(t, expected.GetThreshold(), start.(types.Start).GetThreshold())
	require.Len(t, start.(types.Start).GetAddresses(), len(expected.GetAddresses()))
	require.Len(t, start.(types.Start).GetPublicKeys(), len(expected.GetPublicKeys()))

	_, err = format.Decode(ctx, []byte(`{"Start":{"PublicKeys":[[]]}}`))
	require.EqualError(t, err,
		"couldn't unmarshal public key: invalid Ed25519 curve point")

	badCtx := serde.WithFactory(ctx, types.AddrKey{}, nil)
	_, err = format.Decode(badCtx, []byte(`{"Start":{}}`))
	require.EqualError(t, err, "invalid factory of type '<nil>'")

	// Decode deal messages.
	deal, err := format.Decode(ctx, []byte(`{"Deal":{}}`))
	require.NoError(t, err)
	require.Equal(t, types.NewDeal(0, nil, types.EncryptedDeal{}), deal)

	// Decode response messages.
	resp, err := format.Decode(ctx, []byte(`{"Response":{}}`))
	require.NoError(t, err)
	require.Equal(t, types.NewResponse(0, types.DealerResponse{}), resp)

	// Decode start done messages.
	data = []byte(fmt.Sprintf(`{"StartDone":{"PublicKey":"%s"}}`, testPoint))
	done, err := format.Decode(ctx, data)
	require.NoError(t, err)
	require.IsType(t, types.StartDone{}, done)

	data = []byte(`{"StartDone":{"PublicKey":[]}}`)
	_, err = format.Decode(ctx, data)
	require.EqualError(t, err,
		"couldn't unmarshal public key: invalid Ed25519 curve point")

	// Decode decryption request messages.
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

	// Decode decryption reply messages.
	data = []byte(fmt.Sprintf(`{"DecryptReply":{"I":4,"V":"%s"}}`, testPoint))
	resp, err = format.Decode(ctx, data)
	require.NoError(t, err)
	require.IsType(t, types.DecryptReply{}, resp)

	data = []byte(`{"DecryptReply":{"V":[]}}`)
	_, err = format.Decode(ctx, data)
	require.EqualError(t, err,
		"couldn't unmarshal V: invalid Ed25519 curve point")

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, fake.Err("couldn't deserialize message"))

	_, err = format.Decode(ctx, []byte(`{}`))
	require.EqualError(t, err, "message is empty")
}

func TestMessageFormat_Decode_StartResharing(t *testing.T) {
	format := newMsgFormat()
	ctx := serde.NewContext(fake.ContextEngine{})
	ctx = serde.WithFactory(ctx, types.AddrKey{}, fake.AddressFactory{})

	expected := types.NewStartResharing(
		5,
		6,
		[]mino.Address{fake.NewAddress(0)},
		[]mino.Address{fake.NewAddress(1)},
		[]kyber.Point{suite.Point()},
		[]kyber.Point{suite.Point()},
	)

	data, err := format.Encode(ctx, expected)
	require.NoError(t, err)

	start, err := format.Decode(ctx, data)
	require.NoError(t, err)
	require.Equal(t, expected.GetTNew(), start.(types.StartResharing).GetTNew())
	require.Equal(t, expected.GetTOld(), start.(types.StartResharing).GetTOld())
	require.Len(t, start.(types.StartResharing).GetAddrsNew(), len(expected.GetAddrsNew()))
	require.Len(t, start.(types.StartResharing).GetAddrsOld(), len(expected.GetAddrsOld()))

	badCtx := serde.WithFactory(ctx, types.AddrKey{}, nil)
	_, err = format.Decode(badCtx, []byte(`{"StartResharing":{}}`))
	require.EqualError(t, err, "invalid factory of type '<nil>'")

	_, err = format.Decode(ctx, []byte(`{"StartResharing":{"PubkeysNew":[[]]}}`))
	require.EqualError(t, err,
		"couldn't unmarshal new public key: invalid Ed25519 curve point")

	_, err = format.Decode(ctx, []byte(`{"StartResharing":{"PubkeysOld":[[]]}}`))
	require.EqualError(t, err,
		"couldn't unmarshal old public key: invalid Ed25519 curve point")
}

func TestMessageFormat_Decode_Reshare(t *testing.T) {
	format := newMsgFormat()
	ctx := serde.NewContext(fake.ContextEngine{})
	ctx = serde.WithFactory(ctx, types.AddrKey{}, fake.AddressFactory{})

	expected := types.NewReshare(
		types.NewDeal(3, []byte{}, types.NewEncryptedDeal([]byte{}, []byte{}, []byte{}, []byte{})),
		[]kyber.Point{suite.Point()},
	)

	data, err := format.Encode(ctx, expected)
	require.NoError(t, err)

	reshare, err := format.Decode(ctx, data)
	require.NoError(t, err)
	require.True(t, expected.GetPublicCoeffs()[0].Equal(reshare.(types.Reshare).GetPublicCoeffs()[0]))
	require.Equal(t, expected.GetDeal(), reshare.(types.Reshare).GetDeal())

	_, err = format.Decode(ctx, []byte(`{"Reshare":{"PublicCoeff":[[]]}}`))
	require.EqualError(t, err, "couldn't unmarshal public coeff key: invalid Ed25519 curve point")
}

func TestMessageFormat_Decode_VerifiableDecryptRequest(t *testing.T) {
	format := newMsgFormat()
	ctx := serde.NewContext(fake.ContextEngine{})
	ctx = serde.WithFactory(ctx, types.AddrKey{}, fake.AddressFactory{})

	ct := types.Ciphertext{
		K:    suite.Point().Pick(suite.RandomStream()),
		C:    suite.Point(),
		UBar: suite.Point(),
		E:    suite.Scalar().Pick(suite.RandomStream()),
		F:    suite.Scalar(),
		GBar: suite.Point(),
	}

	expected := types.NewVerifiableDecryptRequest([]types.Ciphertext{ct})

	data, err := format.Encode(ctx, expected)
	require.NoError(t, err)

	reshare, err := format.Decode(ctx, data)
	require.NoError(t, err)

	require.True(t, expected.GetCiphertexts()[0].K.Equal(reshare.(types.VerifiableDecryptRequest).GetCiphertexts()[0].K))
	require.True(t, expected.GetCiphertexts()[0].E.Equal(reshare.(types.VerifiableDecryptRequest).GetCiphertexts()[0].E))

	_, err = format.Decode(ctx, []byte(`{"VerifiableDecryptRequest":{"Ciphertexts":[{}]}}`))
	require.EqualError(t, err, "couldn't unmarshal K: invalid Ed25519 curve point")

	ctJSON := fmt.Sprintf(`{"K":"%s"}`, testPoint)
	_, err = format.Decode(ctx, []byte(fmt.Sprintf(`{"VerifiableDecryptRequest":{"Ciphertexts":[%s]}}`, ctJSON)))
	require.EqualError(t, err, "couldn't unmarshal C: invalid Ed25519 curve point")

	ctJSON = fmt.Sprintf(`{"K":"%s", "C": "%s"}`, testPoint, testPoint)
	_, err = format.Decode(ctx, []byte(fmt.Sprintf(`{"VerifiableDecryptRequest":{"Ciphertexts":[%s]}}`, ctJSON)))
	require.EqualError(t, err, "couldn't unmarshal Ubar: invalid Ed25519 curve point")

	ctJSON = fmt.Sprintf(`{"K": "%s", "C": "%s", "Ubar": "%s"}`, testPoint, testPoint, testPoint)
	_, err = format.Decode(ctx, []byte(fmt.Sprintf(`{"VerifiableDecryptRequest":{"Ciphertexts":[%s]}}`, ctJSON)))
	require.EqualError(t, err, "couldn't unmarshal E: wrong size buffer")

	ctJSON = fmt.Sprintf(`{"K": "%s", "C": "%s", "Ubar": "%s", "E": "%s"}`, testPoint, testPoint, testPoint, testPoint)
	_, err = format.Decode(ctx, []byte(fmt.Sprintf(`{"VerifiableDecryptRequest":{"Ciphertexts":[%s]}}`, ctJSON)))
	require.EqualError(t, err, "couldn't unmarshal F: wrong size buffer")

	ctJSON = fmt.Sprintf(`{"K": "%s", "C": "%s", "Ubar": "%s", "E": "%s", "F": "%s"}`, testPoint, testPoint, testPoint, testPoint, testPoint)
	_, err = format.Decode(ctx, []byte(fmt.Sprintf(`{"VerifiableDecryptRequest":{"Ciphertexts":[%s]}}`, ctJSON)))
	require.EqualError(t, err, "couldn't unmarshal GBar: invalid Ed25519 curve point")
}

func TestMessageFormat_Decode_VerifiableDecryptReply(t *testing.T) {
	format := newMsgFormat()
	ctx := serde.NewContext(fake.ContextEngine{})
	ctx = serde.WithFactory(ctx, types.AddrKey{}, fake.AddressFactory{})

	sp := types.ShareAndProof{
		V:  suite.Point().Pick(suite.RandomStream()),
		I:  int64(0),
		Ui: suite.Point(),
		Ei: suite.Scalar().Pick(suite.RandomStream()),
		Fi: suite.Scalar(),
		Hi: suite.Point(),
	}

	expected := types.NewVerifiableDecryptReply([]types.ShareAndProof{sp})

	data, err := format.Encode(ctx, expected)
	require.NoError(t, err)

	reshare, err := format.Decode(ctx, data)
	require.NoError(t, err)

	require.True(t, expected.GetShareAndProof()[0].V.Equal(reshare.(types.VerifiableDecryptReply).GetShareAndProof()[0].V))
	require.True(t, expected.GetShareAndProof()[0].Ei.Equal(reshare.(types.VerifiableDecryptReply).GetShareAndProof()[0].Ei))

	_, err = format.Decode(ctx, []byte(`{"VerifiableDecryptReply":{"Sp":[{}]}}`))
	require.EqualError(t, err, "couldn't unmarshal V: invalid Ed25519 curve point")

	ctJSON := fmt.Sprintf(`{"V":"%s"}`, testPoint)
	_, err = format.Decode(ctx, []byte(fmt.Sprintf(`{"VerifiableDecryptReply":{"Sp":[%s]}}`, ctJSON)))
	require.EqualError(t, err, "couldn't unmarshal E_i: wrong size buffer")

	ctJSON = fmt.Sprintf(`{"V":"%s", "Ei": "%s"}`, testPoint, testPoint)
	_, err = format.Decode(ctx, []byte(fmt.Sprintf(`{"VerifiableDecryptReply":{"Sp":[%s]}}`, ctJSON)))
	require.EqualError(t, err, "couldn't unmarshal U_i: invalid Ed25519 curve point")

	ctJSON = fmt.Sprintf(`{"V": "%s", "Ei": "%s", "Ui": "%s"}`, testPoint, testPoint, testPoint)
	_, err = format.Decode(ctx, []byte(fmt.Sprintf(`{"VerifiableDecryptReply":{"Sp":[%s]}}`, ctJSON)))
	require.EqualError(t, err, "couldn't unmarshal F_i: wrong size buffer")

	ctJSON = fmt.Sprintf(`{"V": "%s", "Ei": "%s", "Ui": "%s", "Fi": "%s"}`, testPoint, testPoint, testPoint, testPoint)
	_, err = format.Decode(ctx, []byte(fmt.Sprintf(`{"VerifiableDecryptReply":{"Sp":[%s]}}`, ctJSON)))
	require.EqualError(t, err, "couldn't unmarshal H_i: invalid Ed25519 curve point")
}

// -----------------------------------------------------------------------------
// Utility functions

const testPoint = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

type badPoint struct {
	kyber.Point
}

func (p badPoint) MarshalBinary() ([]byte, error) {
	return nil, fake.GetError()
}

type badScallar struct {
	kyber.Scalar
}

func (s badScallar) MarshalBinary() ([]byte, error) {
	return nil, fake.GetError()
}
