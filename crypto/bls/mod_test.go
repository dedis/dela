package bls

import (
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/kyber/v3"
	"golang.org/x/xerrors"
)

func TestPublicKey_MarshalBinary(t *testing.T) {
	signer := NewSigner()

	buffer, err := signer.GetPublicKey().MarshalBinary()
	require.NoError(t, err)
	require.NotEmpty(t, buffer)
}

func TestPublicKey_Verify(t *testing.T) {
	msg := []byte("deadbeef")
	signer := NewSigner()
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
	signerA := NewSigner()
	signerB := NewSigner()
	require.True(t, signerA.GetPublicKey().Equal(signerA.GetPublicKey()))
	require.True(t, signerB.GetPublicKey().Equal(signerB.GetPublicKey()))
	require.False(t, signerA.GetPublicKey().Equal(signerB.GetPublicKey()))
	require.False(t, signerA.GetPublicKey().Equal(fake.PublicKey{}))
}

func TestPublicKey_MarshalText(t *testing.T) {
	signer := NewSigner()
	text, err := signer.GetPublicKey().MarshalText()
	require.NoError(t, err)
	require.Contains(t, string(text), "bls:")

	pk := PublicKey{point: badPoint{}}
	_, err = pk.MarshalText()
	require.EqualError(t, err, "couldn't marshal: oops")
}

func TestPublicKey_String(t *testing.T) {
	signer := NewSigner()
	str := signer.GetPublicKey().(PublicKey).String()
	require.Contains(t, str, "bls:")

	pk := PublicKey{point: badPoint{}}
	str = pk.String()
	require.Equal(t, "bls:malformed_point", str)
}

func TestSignature_MarshalBinary(t *testing.T) {
	f := func(data []byte) bool {
		sig := Signature{data: data}
		buffer, err := sig.MarshalBinary()
		require.NoError(t, err)
		require.Equal(t, data, buffer)

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
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

func TestVerifier_Verify(t *testing.T) {
	f := func(msg []byte) bool {
		signer := NewSigner()
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

	verifier, err := factory.FromAuthority(fake.NewAuthority(2, NewSigner))
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
	signer := NewSigner()

	factory := signer.GetPublicKeyFactory()
	require.NotNil(t, factory)
	require.IsType(t, publicKeyFactory{}, factory)
}

func TestSigner_GetSignatureFactory(t *testing.T) {
	signer := NewSigner()

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
			signer := NewSigner()
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

// -----------------------------------------------------------------------------
// Utility functions

type badPoint struct {
	kyber.Point
}

func (p badPoint) MarshalBinary() ([]byte, error) {
	return nil, xerrors.New("oops")
}
