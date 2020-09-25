package bls

import (
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/kyber/v3"
)

func init() {
	RegisterPublicKeyFormat(fake.GoodFormat, fake.Format{Msg: PublicKey{}})
	RegisterPublicKeyFormat(serde.Format("BAD_TYPE"), fake.Format{Msg: fake.Message{}})
	RegisterPublicKeyFormat(fake.BadFormat, fake.NewBadFormat())
	RegisterSignatureFormat(fake.GoodFormat, fake.Format{Msg: Signature{}})
	RegisterSignatureFormat(serde.Format("BAD_TYPE"), fake.Format{Msg: fake.Message{}})
	RegisterSignatureFormat(fake.BadFormat, fake.NewBadFormat())
}

func TestPublicKey_New(t *testing.T) {
	signer := Generate()
	data, err := signer.GetPublicKey().MarshalBinary()
	require.NoError(t, err)

	pk, err := NewPublicKey(data)
	require.NoError(t, err)
	require.True(t, signer.GetPublicKey().Equal(pk))

	_, err = NewPublicKey(nil)
	require.Error(t, err)
}

func TestPublicKey_MarshalBinary(t *testing.T) {
	signer := Generate()

	buffer, err := signer.GetPublicKey().MarshalBinary()
	require.NoError(t, err)
	require.NotEmpty(t, buffer)
}

func TestPublicKey_Serialize(t *testing.T) {
	pubkey := NewPublicKeyFromPoint(nil)

	data, err := pubkey.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, fake.GetFakeFormatValue(), data)

	_, err = pubkey.Serialize(fake.NewBadContext())
	require.EqualError(t, err, fake.Err("couldn't encode public key"))
}

func TestPublicKey_Verify(t *testing.T) {
	msg := []byte("deadbeef")
	signer := Generate()
	sig, err := signer.Sign(msg)
	require.NoError(t, err)

	err = signer.GetPublicKey().Verify(msg, sig)
	require.NoError(t, err)

	err = signer.GetPublicKey().Verify([]byte{}, sig)
	require.EqualError(t, err, "bls verify failed: bls: invalid signature")

	err = signer.GetPublicKey().Verify(msg, fake.Signature{})
	require.EqualError(t, err, "invalid signature type 'fake.Signature'")
}

func TestPublicKey_Equal(t *testing.T) {
	signerA := Generate()
	signerB := Generate()
	require.True(t, signerA.GetPublicKey().Equal(signerA.GetPublicKey()))
	require.True(t, signerB.GetPublicKey().Equal(signerB.GetPublicKey()))
	require.False(t, signerA.GetPublicKey().Equal(signerB.GetPublicKey()))
	require.False(t, signerA.GetPublicKey().Equal(fake.PublicKey{}))
}

func TestPublicKey_MarshalText(t *testing.T) {
	signer := Generate()
	text, err := signer.GetPublicKey().MarshalText()
	require.NoError(t, err)
	require.Regexp(t, "^bls:", string(text))

	pk := PublicKey{point: badPoint{}}
	_, err = pk.MarshalText()
	require.EqualError(t, err, fake.Err("couldn't marshal"))
}

func TestPublicKey_String(t *testing.T) {
	signer := Generate()
	str := signer.GetPublicKey().(PublicKey).String()
	require.Contains(t, str, "bls:")

	pk := PublicKey{point: badPoint{}}
	str = pk.String()
	require.Equal(t, "bls:malformed_point", str)
}

func TestPublicKeyFactory_Deserialize(t *testing.T) {
	factory := NewPublicKeyFactory()

	msg, err := factory.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, PublicKey{}, msg)

	_, err = factory.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, fake.Err("couldn't decode public key"))
}

func TestPublicKeyFactory_PublicKeyOf(t *testing.T) {
	factory := NewPublicKeyFactory()

	pk, err := factory.PublicKeyOf(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, PublicKey{}, pk)

	_, err = factory.PublicKeyOf(fake.NewBadContext(), nil)
	require.EqualError(t, err, fake.Err("couldn't decode public key"))

	_, err = factory.PublicKeyOf(fake.NewContextWithFormat(serde.Format("BAD_TYPE")), nil)
	require.EqualError(t, err, "invalid public key of type 'fake.Message'")
}

func TestPublicKeyFactory_FromBytes(t *testing.T) {
	factory := NewPublicKeyFactory()

	point := suite.Point()
	data, err := point.MarshalBinary()
	require.NoError(t, err)

	pk, err := factory.FromBytes(data)
	require.NoError(t, err)
	require.True(t, pk.(PublicKey).point.Equal(point))

	_, err = factory.FromBytes(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to unmarshal key: ")
}

func TestSignature_MarshalBinary(t *testing.T) {
	f := func(data []byte) bool {
		sig := NewSignature(data)
		buffer, err := sig.MarshalBinary()
		require.NoError(t, err)
		require.Equal(t, data, buffer)

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestSignature_Serialize(t *testing.T) {
	sig := Signature{}

	data, err := sig.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, fake.GetFakeFormatValue(), data)

	_, err = sig.Serialize(fake.NewBadContext())
	require.EqualError(t, err, fake.Err("couldn't encode signature"))
}

func TestSignature_Equal(t *testing.T) {
	f := func(data []byte) bool {
		sig := Signature{data: data}
		require.True(t, sig.Equal(Signature{data: data}))

		buffer := append(append([]byte{}, data...), 0xaa)
		require.False(t, sig.Equal(Signature{data: buffer}))

		require.False(t, sig.Equal(fake.Signature{}))

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestSignature_String(t *testing.T) {
	sig := Signature{data: []byte{1, 2, 3}}
	require.Equal(t, "bls:010203", sig.String())
}

func TestSignatureFactory_Deserialize(t *testing.T) {
	factory := NewSignatureFactory()

	msg, err := factory.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, Signature{}, msg)

	_, err = factory.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, fake.Err("couldn't decode signature"))
}

func TestSignatureFactory_SignatureOf(t *testing.T) {
	factory := NewSignatureFactory()

	sig, err := factory.SignatureOf(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, Signature{}, sig)

	_, err = factory.SignatureOf(fake.NewBadContext(), nil)
	require.EqualError(t, err, fake.Err("couldn't decode signature"))

	_, err = factory.SignatureOf(fake.NewContextWithFormat(serde.Format("BAD_TYPE")), nil)
	require.EqualError(t, err, "invalid signature of type 'fake.Message'")
}

func TestVerifier_Verify(t *testing.T) {
	f := func(msg []byte) bool {
		signer := Generate()
		sig, err := signer.Sign(msg)
		require.NoError(t, err)

		verifier := newVerifier(
			[]kyber.Point{signer.GetPublicKey().(PublicKey).point},
		)
		err = verifier.Verify(msg, sig)
		require.NoError(t, err)

		err = verifier.Verify(append([]byte{1}, msg...), sig)
		require.Error(t, err)

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestVerifierFactory_FromAuthority(t *testing.T) {
	factory := verifierFactory{}

	verifier, err := factory.FromAuthority(fake.NewAuthority(2, Generate))
	require.NoError(t, err)
	require.IsType(t, blsVerifier{}, verifier)
	require.Len(t, verifier.(blsVerifier).points, 2)
	require.NotNil(t, verifier.(blsVerifier).points[0])
	require.NotNil(t, verifier.(blsVerifier).points[1])

	_, err = factory.FromAuthority(nil)
	require.EqualError(t, err, "authority is nil")

	_, err = factory.FromAuthority(fake.NewAuthority(2, fake.NewSigner))
	require.EqualError(t, err, "invalid public key type: fake.PublicKey")
}

func TestVerifierFactory_FromArray(t *testing.T) {
	factory := verifierFactory{}

	verifier, err := factory.FromArray([]crypto.PublicKey{PublicKey{}})
	require.NoError(t, err)
	require.Len(t, verifier.(blsVerifier).points, 1)

	verifier, err = factory.FromArray(nil)
	require.NoError(t, err)
	require.Empty(t, verifier.(blsVerifier).points)

	_, err = factory.FromArray([]crypto.PublicKey{fake.PublicKey{}})
	require.EqualError(t, err, "invalid public key type: fake.PublicKey")
}

func TestSigner_GetVerifierFactory(t *testing.T) {
	signer := NewSigner()

	factory := signer.GetVerifierFactory()
	require.NotNil(t, factory)
	require.IsType(t, verifierFactory{}, factory)
}

func TestSigner_GetPublicKeyFactory(t *testing.T) {
	signer := Generate()

	factory := signer.GetPublicKeyFactory()
	require.NotNil(t, factory)
	require.IsType(t, publicKeyFactory{}, factory)
}

func TestSigner_GetSignatureFactory(t *testing.T) {
	signer := Generate()

	factory := signer.GetSignatureFactory()
	require.NotNil(t, factory)
	require.IsType(t, signatureFactory{}, factory)
}

func TestSigner_Sign(t *testing.T) {
	signer := NewSigner()
	f := func(msg []byte) bool {
		sig, err := signer.Sign(msg)
		require.NoError(t, err)

		verifier, err := signer.GetVerifierFactory().FromArray(
			[]crypto.PublicKey{signer.GetPublicKey()},
		)
		require.NoError(t, err)
		require.NoError(t, verifier.Verify(msg, sig))

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestSigner_Aggregate(t *testing.T) {
	N := 3

	f := func(msg []byte) bool {
		signatures := make([]crypto.Signature, N)
		pubkeys := make([]crypto.PublicKey, N)
		for i := 0; i < N; i++ {
			signer := Generate()
			pubkeys[i] = signer.GetPublicKey()
			sig, err := signer.Sign(msg)
			require.NoError(t, err)
			signatures[i] = sig
		}

		signer := NewSigner()
		agg, err := signer.Aggregate(signatures...)
		require.NoError(t, err)

		verifier, err := signer.GetVerifierFactory().FromArray(pubkeys)
		require.NoError(t, err)
		err = verifier.Verify(msg, agg)
		require.NoError(t, err)

		return agg != nil
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestSigner_MarshalBinary(t *testing.T) {
	signer := NewSigner()

	sig, err := signer.Sign([]byte{1, 2, 3})
	require.NoError(t, err)

	data, err := signer.MarshalBinary()
	require.NoError(t, err)

	next, err := NewSignerFromBytes(data)
	require.NoError(t, err)
	require.NoError(t, next.GetPublicKey().Verify([]byte{1, 2, 3}, sig))

	signer.private = badScalar{}
	_, err = signer.MarshalBinary()
	require.EqualError(t, err, fake.Err("while marshaling scalar"))

	_, err = NewSignerFromBytes(nil)
	require.EqualError(t, err, "while unmarshaling scalar: UnmarshalBinary: wrong size buffer")
}

// -----------------------------------------------------------------------------
// Utility functions

type badPoint struct {
	kyber.Point
}

func (p badPoint) MarshalBinary() ([]byte, error) {
	return nil, fake.GetError()
}

type badScalar struct {
	kyber.Scalar
}

func (s badScalar) MarshalBinary() ([]byte, error) {
	return nil, fake.GetError()
}
