package threshold

import (
	"testing"

	"github.com/golang/protobuf/ptypes"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/encoding"
	"go.dedis.ch/fabric/internal/testing/fake"
	"go.dedis.ch/fabric/mino"
)

func TestSignature_HasBit(t *testing.T) {
	sig := &Signature{mask: []byte{0b00000010, 0b10000001}}

	require.True(t, sig.HasBit(1))
	require.True(t, sig.HasBit(8))
	require.True(t, sig.HasBit(15))
	require.False(t, sig.HasBit(16))
	require.False(t, sig.HasBit(-1))
}

func TestSignature_GetIndices(t *testing.T) {
	sig := &Signature{mask: []byte{0x0c, 0x01}}

	require.Equal(t, []int{2, 3, 8}, sig.GetIndices())
}

func TestSignature_Merge(t *testing.T) {
	sig := &Signature{}

	err := sig.Merge(fake.NewSigner(), 2, fake.Signature{})
	require.NoError(t, err)
	require.NotNil(t, sig.agg)

	err = sig.Merge(fake.NewSigner(), 1, fake.Signature{})
	require.NoError(t, err)

	err = sig.Merge(fake.NewSigner(), 1, fake.Signature{})
	require.EqualError(t, err, "index 1 already merged")

	err = sig.Merge(fake.NewBadSigner(), 0, fake.Signature{})
	require.EqualError(t, err, "couldn't aggregate: fake error")
	require.Equal(t, []byte{0b00000110}, sig.mask)
}

func TestSignature_SetBit(t *testing.T) {
	sig := &Signature{}

	sig.setBit(-1)
	require.Nil(t, sig.mask)

	sig.setBit(8)
	require.Len(t, sig.mask, 2)
	require.Equal(t, sig.mask[1], uint8(1))

	sig.setBit(9)
	require.Len(t, sig.mask, 2)
	require.Equal(t, sig.mask[1], uint8(3))
}

func TestSignature_Pack(t *testing.T) {
	sig := &Signature{
		agg:  fake.Signature{},
		mask: []byte{0xff},
	}

	pb, err := sig.Pack(encoding.NewProtoEncoder())
	require.NoError(t, err)
	require.NotNil(t, pb)
	require.Equal(t, []byte{0xff}, pb.(*SignatureProto).GetMask())

	_, err = sig.Pack(fake.BadPackAnyEncoder{})
	require.EqualError(t, err, "couldn't pack signature: fake error")
}

func TestSignature_MarshalBinary(t *testing.T) {
	sig := &Signature{
		agg:  fake.Signature{},
		mask: []byte{0xff},
	}

	buffer, err := sig.MarshalBinary()
	require.NoError(t, err)
	require.Equal(t, []byte{fake.SignatureByte, 0xff}, buffer)

	sig.agg = fake.NewBadSignature()
	_, err = sig.MarshalBinary()
	require.EqualError(t, err, "couldn't marshal signature: fake error")
}

func TestSignature_Equal(t *testing.T) {
	sig := &Signature{
		agg:  fake.Signature{},
		mask: []byte{0xff},
	}

	require.True(t, sig.Equal(sig))
	require.False(t, sig.Equal(nil))
}

func TestSignatureFactory_FromProto(t *testing.T) {
	factory := signatureFactory{
		encoder:    encoding.NewProtoEncoder(),
		sigFactory: fake.SignatureFactory{},
	}

	pb := &SignatureProto{}
	pbAny, err := ptypes.MarshalAny(pb)
	require.NoError(t, err)

	sig, err := factory.FromProto(pb)
	require.NoError(t, err)
	require.NotNil(t, sig)

	sig, err = factory.FromProto(pbAny)
	require.NoError(t, err)
	require.NotNil(t, sig)

	_, err = factory.FromProto(nil)
	require.EqualError(t, err, "invalid signature type '<nil>'")

	factory.sigFactory = fake.NewBadSignatureFactory()
	_, err = factory.FromProto(pb)
	require.EqualError(t, err, "couldn't decode aggregation: fake error")

	factory.encoder = fake.BadUnmarshalAnyEncoder{}
	_, err = factory.FromProto(pbAny)
	require.EqualError(t, err, "couldn't unmarshal message: fake error")
}

func TestVerifier_Verify(t *testing.T) {
	call := &fake.Call{}

	verifier := newVerifier(fake.NewAuthority(3, fake.NewSigner), fake.NewVerifier(call))

	err := verifier.Verify([]byte{0xff}, &Signature{mask: []byte{0x3}})
	require.NoError(t, err)
	require.Equal(t, 1, call.Len())
	require.Equal(t, 2, call.Get(0, 0).(mino.Players).Len())

	err = verifier.Verify([]byte{}, nil)
	require.EqualError(t, err, "invalid signature type '<nil>' != '*threshold.Signature'")

	verifier.factory = fake.NewBadVerifierFactory()
	err = verifier.Verify([]byte{}, &Signature{})
	require.EqualError(t, err, "couldn't make verifier: fake error")

	verifier.factory = fake.NewVerifierFactory(fake.NewBadVerifier())
	err = verifier.Verify([]byte{}, &Signature{})
	require.EqualError(t, err, "invalid signature: fake error")
}
