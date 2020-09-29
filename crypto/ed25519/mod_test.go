package ed25519

import (
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/sign/schnorr"
	"go.dedis.ch/kyber/v3/util/key"
)

func init() {
	RegisterPublicKeyFormat(fake.GoodFormat, fake.Format{Msg: PublicKey{}})
	RegisterPublicKeyFormat(fake.BadFormat, fake.NewBadFormat())
	RegisterPublicKeyFormat("BAD_POINT", fake.Format{Msg: fake.Message{}})

	RegisterSignatureFormat(fake.GoodFormat, fake.Format{Msg: Signature{}})
	RegisterSignatureFormat(fake.BadFormat, fake.NewBadFormat())
	RegisterSignatureFormat("BAD_SIG", fake.Format{Msg: fake.Message{}})
}

func TestPublicKey_New(t *testing.T) {
	point := suite.Point()
	pointBuf, err := point.MarshalBinary()
	require.NoError(t, err)

	pubKey, err := NewPublicKey(pointBuf)
	require.NoError(t, err)

	require.True(t, pubKey.GetPoint().Equal(point))

	_, err = NewPublicKey([]byte{})
	require.EqualError(t, err, "couldn't unmarshal point: invalid Ed25519 curve point")
}

func TestPublicKey_NewFromPoint(t *testing.T) {
	point := suite.Point()
	pk := NewPublicKeyFromPoint(point)
	require.True(t, pk.GetPoint().Equal(point))
}

func TestPublicKey_MarshalBinary(t *testing.T) {
	point := suite.Point()
	pointBuf, err := point.MarshalBinary()
	require.NoError(t, err)

	pk := PublicKey{point: point}
	pointBuf2, err := pk.MarshalBinary()
	require.NoError(t, err)

	require.Equal(t, pointBuf, pointBuf2)
}

func TestPublicKey_Serialize(t *testing.T) {
	pk := PublicKey{}
	ctx := fake.NewContext()

	pkBuf, err := pk.Serialize(ctx)
	require.NoError(t, err)
	require.Equal(t, fake.GetFakeFormatValue(), pkBuf)

	_, err = pk.Serialize(fake.NewBadContext())
	require.EqualError(t, err, fake.Err("couldn't encode public key"))
}

func TestPublicKey_Verify(t *testing.T) {
	privKey := suite.Scalar().Pick(suite.RandomStream())
	pubKey := suite.Point().Mul(privKey, nil)
	pk := PublicKey{point: pubKey}

	msg := []byte("hello")
	signature, err := schnorr.Sign(suite, privKey, msg)
	require.NoError(t, err)

	err = pk.Verify(msg, Signature{data: signature})
	require.NoError(t, err)

	err = pk.Verify(msg, fake.NewBadSignature())
	require.EqualError(t, err, "invalid signature type 'fake.Signature'")

	err = pk.Verify(msg, Signature{data: []byte{}})
	// the second error part depends on kyber implementation
	require.Regexp(t, "^schnorr verify failed: ", err)
}

func TestPublicKey_Equal(t *testing.T) {
	point := suite.Point()
	pk := PublicKey{point: point}
	pk2 := PublicKey{point: point}

	require.True(t, pk.Equal(pk2))
	require.False(t, pk.Equal(fake.NewBadPublicKey()))

	point2 := suite.Point().Pick(suite.RandomStream())
	pk2 = PublicKey{point: point2}

	require.False(t, pk.Equal(pk2))
}

func TestPublicKey_MarshalText(t *testing.T) {
	point := suite.Point()
	pk := PublicKey{point: point}

	res, err := pk.MarshalText()
	require.NoError(t, err)
	require.Regexp(t, "^schnorr:", string(res))

	pk.point = badPoint{}
	_, err = pk.MarshalText()
	require.EqualError(t, err, fake.Err("couldn't marshal"))
}

func TestPublicKey_GetPoint(t *testing.T) {
	point := suite.Point()
	pk := PublicKey{point: point}

	require.True(t, point.Equal(pk.GetPoint()))
}

func TestPublicKey_String(t *testing.T) {
	point := suite.Point()
	pk := PublicKey{point: point}

	res := pk.String()
	require.Regexp(t, "^schnorr:[a-f0-9]{16}$", res)

	pk.point = badPoint{}
	res = pk.String()
	require.Equal(t, "schnorr:malformed_point", res)
}

func TestSignature_New(t *testing.T) {
	data := []byte("hello")
	sig := NewSignature(data)
	require.Equal(t, data, sig.data)
}

func TestSignature_MarshalBinary(t *testing.T) {
	data := []byte("hello")
	sig := NewSignature(data)

	buf, err := sig.MarshalBinary()
	require.NoError(t, err)
	require.Equal(t, data, buf)
}

func TestSignature_Serialize(t *testing.T) {
	data := []byte("hello")
	sig := NewSignature(data)

	ctx := fake.NewContext()
	buf, err := sig.Serialize(ctx)
	require.NoError(t, err)
	require.Equal(t, fake.GetFakeFormatValue(), buf)

	ctx = fake.NewBadContext()
	_, err = sig.Serialize(ctx)
	require.EqualError(t, err, fake.Err("couldn't encode signature"))
}

func TestSignature_Equal(t *testing.T) {
	data := []byte("hello")
	sig := NewSignature(data)
	sig2 := NewSignature(data)

	require.True(t, sig.Equal(sig2))
	require.False(t, sig.Equal(fake.NewBadSignature()))

	data2 := []byte("world")
	sig2 = NewSignature(data2)

	require.False(t, sig.Equal(sig2))
}

func TestPublicKeyFactory_New(t *testing.T) {
	pkf := NewPublicKeyFactory()
	require.IsType(t, publicKeyFactory{}, pkf)
}

func TestPublicKeyFactory_Deserialize(t *testing.T) {
	pkf := NewPublicKeyFactory()
	ctx := fake.NewContext()

	msg, err := pkf.Deserialize(ctx, nil)
	require.NoError(t, err)
	require.IsType(t, PublicKey{}, msg)
}

func TestPublicKeyFactory_PublicKeyOf(t *testing.T) {
	pkf := NewPublicKeyFactory()
	ctx := fake.NewContext()

	msg, err := pkf.PublicKeyOf(ctx, nil)
	require.NoError(t, err)
	require.IsType(t, PublicKey{}, msg)

	ctx = fake.NewBadContext()
	_, err = pkf.PublicKeyOf(ctx, nil)
	require.EqualError(t, err, fake.Err("couldn't decode public key"))

	ctx = fake.NewContextWithFormat("BAD_POINT")
	_, err = pkf.PublicKeyOf(ctx, nil)
	require.EqualError(t, err, "invalid public key of type 'fake.Message'")
}

func TestPublicKeyFactory_FromBytes(t *testing.T) {
	fac := NewPublicKeyFactory()

	point := suite.Point()
	data, err := point.MarshalBinary()
	require.NoError(t, err)

	pk, err := fac.FromBytes(data)
	require.NoError(t, err)
	require.True(t, pk.(PublicKey).point.Equal(point))

	_, err = fac.FromBytes(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to unmarshal the key: ")
}

func TestSignatureFactory_New(t *testing.T) {
	sf := NewSignatureFactory()
	require.IsType(t, signatureFactory{}, sf)
}

func TestSignatureFactory_Deserialize(t *testing.T) {
	sf := NewSignatureFactory()
	ctx := fake.NewContext()

	msg, err := sf.Deserialize(ctx, nil)
	require.NoError(t, err)
	require.IsType(t, Signature{}, msg)
}

func TestSignatureFactory_SignatureOf(t *testing.T) {
	sf := NewSignatureFactory()
	ctx := fake.NewContext()

	msg, err := sf.SignatureOf(ctx, nil)
	require.NoError(t, err)
	require.IsType(t, Signature{}, msg)

	ctx = fake.NewBadContext()
	_, err = sf.SignatureOf(ctx, nil)
	require.EqualError(t, err, fake.Err("couldn't decode signature"))

	ctx = fake.NewContextWithFormat("BAD_SIG")
	_, err = sf.SignatureOf(ctx, nil)
	require.EqualError(t, err, "invalid signature of type 'fake.Message'")
}

func TestSigner_New(t *testing.T) {
	signer := NewSigner()
	require.IsType(t, Signer{}, signer)
}

func TestSigner_GetPublicKeyFactory(t *testing.T) {
	signer := NewSigner()
	factory := signer.GetPublicKeyFactory()
	require.IsType(t, publicKeyFactory{}, factory)
}

func TestSigner_GetSignatureFactory(t *testing.T) {
	signer := NewSigner()
	factory := signer.GetSignatureFactory()
	require.IsType(t, signatureFactory{}, factory)
}

func TestSigner_GetPublicKey(t *testing.T) {
	kp := key.NewKeyPair(suite)
	signer := Signer{keyPair: kp}

	pk := NewPublicKeyFromPoint(kp.Public)

	pk2 := signer.GetPublicKey()
	require.True(t, pk.Equal(pk2))
}

func TestSigner_GetPrivateKey(t *testing.T) {
	kp := key.NewKeyPair(suite)
	signer := Signer{keyPair: kp}

	secret := signer.GetPrivateKey()
	require.True(t, secret.Equal(kp.Private))
}

func TestSigner_Sign(t *testing.T) {
	kp := key.NewKeyPair(suite)
	signer := Signer{keyPair: kp}

	f := func(msg []byte) bool {
		signature, err := signer.Sign(msg)
		require.NoError(t, err)

		signData, err := signature.MarshalBinary()
		require.NoError(t, err)

		err = schnorr.Verify(suite, kp.Public, msg, signData)
		require.NoError(t, err)

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

// -----------------------------------------------------------------------------
// Utility functions

type badPoint struct {
	kyber.Point
}

func (p badPoint) MarshalBinary() ([]byte, error) {
	return nil, fake.GetError()
}
