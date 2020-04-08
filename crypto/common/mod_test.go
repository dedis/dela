package common

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"golang.org/x/xerrors"
)

func TestPublicKeyFactory_Register(t *testing.T) {
	factory := NewPublicKeyFactory()

	length := len(factory.factories)

	factory.Register(&empty.Empty{}, fakePublicKeyFactory{})
	require.Len(t, factory.factories, length+1)

	factory.Register(&empty.Empty{}, fakePublicKeyFactory{})
	require.Len(t, factory.factories, length+1)
}

func TestPublicKeyFactory_FromProto(t *testing.T) {
	factory := NewPublicKeyFactory()
	factory.Register(&empty.Empty{}, fakePublicKeyFactory{})

	publicKey, err := factory.FromProto(&empty.Empty{})
	require.NoError(t, err)
	require.NotNil(t, publicKey)

	pbany, err := ptypes.MarshalAny(&empty.Empty{})
	require.NoError(t, err)
	_, err = factory.FromProto(pbany)
	require.NoError(t, err)

	_, err = factory.FromProto(&wrappers.StringValue{})
	require.EqualError(t, err, "couldn't find factory for '*wrappers.StringValue'")

	factory.encoder = badUnmarshalDynEncoder{}
	_, err = factory.FromProto(pbany)
	require.EqualError(t, err, "couldn't decode message: oops")
}

func TestSignatureFactory_Register(t *testing.T) {
	factory := NewSignatureFactory()

	length := len(factory.factories)

	factory.Register(&empty.Empty{}, fakeSignatureFactory{})
	require.Len(t, factory.factories, length+1)

	factory.Register(&empty.Empty{}, fakeSignatureFactory{})
	require.Len(t, factory.factories, length+1)
}

func TestSignatureFactory_FromProto(t *testing.T) {
	factory := NewSignatureFactory()
	factory.Register(&empty.Empty{}, fakeSignatureFactory{})

	sig, err := factory.FromProto(&empty.Empty{})
	require.NoError(t, err)
	require.NotNil(t, sig)

	sigany, err := ptypes.MarshalAny(&empty.Empty{})
	require.NoError(t, err)
	_, err = factory.FromProto(sigany)
	require.NoError(t, err)

	_, err = factory.FromProto(&wrappers.BoolValue{})
	require.EqualError(t, err, "couldn't find factory for '*wrappers.BoolValue'")

	factory.encoder = badUnmarshalDynEncoder{}
	_, err = factory.FromProto(sigany)
	require.EqualError(t, err, "couldn't decode message: oops")
}

// -----------------------------------------------------------------------------
// Utility functions

type badUnmarshalDynEncoder struct {
	encoding.ProtoEncoder
}

func (e badUnmarshalDynEncoder) UnmarshalDynamicAny(*any.Any) (proto.Message, error) {
	return nil, xerrors.New("oops")
}

type fakePublicKey struct {
	crypto.PublicKey
}

type fakePublicKeyFactory struct {
	crypto.PublicKeyFactory
}

func (f fakePublicKeyFactory) FromProto(proto.Message) (crypto.PublicKey, error) {
	return fakePublicKey{}, nil
}

type fakeSignature struct {
	crypto.Signature
}

type fakeSignatureFactory struct {
	crypto.SignatureFactory
}

func (f fakeSignatureFactory) FromProto(proto.Message) (crypto.Signature, error) {
	return fakeSignature{}, nil
}
