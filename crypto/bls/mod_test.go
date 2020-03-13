package bls

import (
	"bytes"
	"testing"
	"testing/quick"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	internal "go.dedis.ch/fabric/internal/testing"
	"go.dedis.ch/kyber/v3"
	"golang.org/x/xerrors"
)

func TestMessages(t *testing.T) {
	messages := []proto.Message{
		&PublicKeyProto{},
		&SignatureProto{},
	}

	for _, m := range messages {
		internal.CoverProtoMessage(t, m)
	}
}

func TestPublicKey_MarshalBinary(t *testing.T) {
	signer := NewSigner()

	buffer, err := signer.GetPublicKey().MarshalBinary()
	require.NoError(t, err)
	require.NotEmpty(t, buffer)
}

type fakePoint struct {
	kyber.Point
}

func (p fakePoint) MarshalBinary() ([]byte, error) {
	return nil, xerrors.New("oops")
}

func TestPublicKey_Pack(t *testing.T) {
	f := func() bool {
		signer := NewSigner()
		packed, err := signer.GetPublicKey().Pack()
		require.NoError(t, err)

		pubkey, err := signer.GetPublicKeyFactory().FromProto(packed)
		require.NoError(t, err)
		require.True(t, pubkey.Equal(signer.GetPublicKey()))

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)

	pubkey := publicKey{point: fakePoint{}}
	_, err = pubkey.Pack()
	require.EqualError(t, err, "couldn't encode point: oops")
}

type fakePublicKey struct {
	crypto.PublicKey
}

func TestPublicKey_Equal(t *testing.T) {
	f := func() bool {
		signerA := NewSigner()
		signerB := NewSigner()
		require.True(t, signerA.GetPublicKey().Equal(signerA.GetPublicKey()))
		require.True(t, signerB.GetPublicKey().Equal(signerB.GetPublicKey()))
		require.False(t, signerA.GetPublicKey().Equal(signerB.GetPublicKey()))
		require.False(t, signerA.GetPublicKey().Equal(fakePublicKey{}))

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestSignature_MarshalBinary(t *testing.T) {
	f := func(data []byte) bool {
		sig := signature{data: data}
		buffer, err := sig.MarshalBinary()
		require.NoError(t, err)
		require.Equal(t, data, buffer)

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestSignature_Pack(t *testing.T) {
	f := func(data []byte) bool {
		sig := signature{data: data}
		packed, err := sig.Pack()
		require.NoError(t, err)

		pb, ok := packed.(*SignatureProto)
		require.True(t, ok)

		return bytes.Equal(data, pb.GetData())
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

type fakeSignature struct {
	crypto.Signature
}

func TestSignature_Equal(t *testing.T) {
	f := func(data []byte) bool {
		sig := signature{data: data}
		require.True(t, sig.Equal(signature{data: data}))

		buffer := append(append([]byte{}, data...), 0xaa)
		require.False(t, sig.Equal(signature{data: buffer}))

		require.False(t, sig.Equal(fakeSignature{}))

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

type fakeProtoEncoder struct {
	encoding.ProtoMarshaler
}

func (e fakeProtoEncoder) UnmarshalAny(*any.Any, proto.Message) error {
	return xerrors.New("oops")
}

func TestPublicKeyFactory_FromProto(t *testing.T) {
	defer func() { protoenc = encoding.NewProtoEncoder() }()

	factory := publicKeyFactory{}

	signer := NewSigner()
	packed, err := signer.GetPublicKey().Pack()
	require.NoError(t, err)

	pubkey, err := factory.FromProto(packed)
	require.NoError(t, err)
	require.True(t, signer.GetPublicKey().Equal(pubkey))

	packedAny, err := ptypes.MarshalAny(packed)
	require.NoError(t, err)

	pubkey, err = factory.FromProto(packedAny)
	require.NoError(t, err)
	require.True(t, signer.GetPublicKey().Equal(pubkey))

	_, err = factory.FromProto(&empty.Empty{})
	require.EqualError(t, err, "invalid public key type '*empty.Empty'")

	protoenc = fakeProtoEncoder{}
	_, err = factory.FromProto(packedAny)
	require.Error(t, err)
	require.True(t, xerrors.Is(err, encoding.NewAnyDecodingError((*PublicKeyProto)(nil), nil)))

	_, err = factory.FromProto(&PublicKeyProto{Data: []byte{}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to unmarshal point: ")
}

func TestSignatureFactory_FromProto(t *testing.T) {
	defer func() { protoenc = encoding.NewProtoEncoder() }()

	factory := signatureFactory{}

	signer := NewSigner()
	sig, err := signer.Sign([]byte{1})
	require.NoError(t, err)
	packed, err := sig.Pack()
	require.NoError(t, err)

	decoded, err := factory.FromProto(packed)
	require.NoError(t, err)
	require.True(t, sig.Equal(decoded))

	packedAny, err := ptypes.MarshalAny(packed)
	require.NoError(t, err)
	decoded, err = factory.FromProto(packedAny)
	require.NoError(t, err)
	require.True(t, sig.Equal(decoded))

	_, err = factory.FromProto(&empty.Empty{})
	require.EqualError(t, err, "invalid signature type '*empty.Empty'")

	protoenc = fakeProtoEncoder{}
	_, err = factory.FromProto(packedAny)
	require.Error(t, err)
	require.True(t, xerrors.Is(err, encoding.NewAnyDecodingError((*SignatureProto)(nil), nil)))
}

func TestVerifier_Verify(t *testing.T) {
	f := func(msg []byte) bool {
		signer := NewSigner()
		sig, err := signer.Sign(msg)
		require.NoError(t, err)

		verifier := newVerifier(
			[]kyber.Point{signer.GetPublicKey().(publicKey).point},
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

type fakeIterator struct {
	count int
	bad   bool
}

func (i *fakeIterator) HasNext() bool {
	return i.count > 0
}

func (i *fakeIterator) GetNext() crypto.PublicKey {
	i.count--
	if i.bad {
		return fakePublicKey{}
	}

	return publicKey{point: suite.Point()}
}

func TestVerifierFactory_FromIterator(t *testing.T) {
	factory := verifierFactory{}

	verifier, err := factory.FromIterator(&fakeIterator{count: 2})
	require.NoError(t, err)
	require.IsType(t, blsVerifier{}, verifier)
	require.Len(t, verifier.(blsVerifier).points, 2)
	require.NotNil(t, verifier.(blsVerifier).points[0])
	require.NotNil(t, verifier.(blsVerifier).points[1])

	verifier, err = factory.FromIterator(nil)
	require.EqualError(t, err, "iterator is nil")

	verifier, err = factory.FromIterator(&fakeIterator{count: 1, bad: true})
	require.EqualError(t, err, "invalid public key type: bls.fakePublicKey")
}

func TestVerifierFactory_FromArray(t *testing.T) {
	factory := verifierFactory{}

	verifier, err := factory.FromArray([]crypto.PublicKey{publicKey{}})
	require.NoError(t, err)
	require.Len(t, verifier.(blsVerifier).points, 1)

	verifier, err = factory.FromArray(nil)
	require.NoError(t, err)
	require.Empty(t, verifier.(blsVerifier).points)

	verifier, err = factory.FromArray([]crypto.PublicKey{fakePublicKey{}})
	require.EqualError(t, err, "invalid public key type: bls.fakePublicKey")
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
		err = verifier.Verify(msg, sig)
		return err == nil
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
