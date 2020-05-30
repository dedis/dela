package common

import (
	"testing"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestPublicKeyFactory_Register(t *testing.T) {
	factory := NewPublicKeyFactory()

	length := len(factory.factories)

	factory.Register(&empty.Empty{}, fake.PublicKeyFactory{})
	require.Len(t, factory.factories, length+1)

	factory.Register(&empty.Empty{}, fake.PublicKeyFactory{})
	require.Len(t, factory.factories, length+1)
}

func TestPublicKeyFactory_FromProto(t *testing.T) {
	factory := NewPublicKeyFactory()
	factory.Register(&empty.Empty{}, fake.PublicKeyFactory{})

	publicKey, err := factory.FromProto(&empty.Empty{})
	require.NoError(t, err)
	require.NotNil(t, publicKey)

	pbany, err := ptypes.MarshalAny(&empty.Empty{})
	require.NoError(t, err)
	_, err = factory.FromProto(pbany)
	require.NoError(t, err)

	_, err = factory.FromProto(&wrappers.StringValue{})
	require.EqualError(t, err, "couldn't find factory for '*wrapperspb.StringValue'")

	factory.encoder = fake.BadUnmarshalDynEncoder{}
	_, err = factory.FromProto(pbany)
	require.EqualError(t, err, "couldn't decode message: fake error")
}

func TestSignatureFactory_Register(t *testing.T) {
	factory := NewSignatureFactory()

	length := len(factory.factories)

	factory.Register(&empty.Empty{}, fake.SignatureFactory{})
	require.Len(t, factory.factories, length+1)

	factory.Register(&empty.Empty{}, fake.SignatureFactory{})
	require.Len(t, factory.factories, length+1)
}

func TestSignatureFactory_FromProto(t *testing.T) {
	factory := NewSignatureFactory()
	factory.Register(&empty.Empty{}, fake.SignatureFactory{})

	sig, err := factory.FromProto(&empty.Empty{})
	require.NoError(t, err)
	require.NotNil(t, sig)

	sigany, err := ptypes.MarshalAny(&empty.Empty{})
	require.NoError(t, err)
	_, err = factory.FromProto(sigany)
	require.NoError(t, err)

	_, err = factory.FromProto(&wrappers.BoolValue{})
	require.EqualError(t, err, "couldn't find factory for '*wrapperspb.BoolValue'")

	factory.encoder = fake.BadUnmarshalDynEncoder{}
	_, err = factory.FromProto(sigany)
	require.EqualError(t, err, "couldn't decode message: fake error")
}
