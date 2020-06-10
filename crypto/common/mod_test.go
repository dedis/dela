package common

import (
	"testing"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde/json"
	"golang.org/x/xerrors"
)

func TestPublicKeyFactory_Register(t *testing.T) {
	factory := NewPublicKeyFactory()

	length := len(factory.deprecated)

	factory.Register(&empty.Empty{}, fake.PublicKeyFactory{})
	require.Len(t, factory.deprecated, length+1)

	factory.Register(&empty.Empty{}, fake.PublicKeyFactory{})
	require.Len(t, factory.deprecated, length+1)
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
	require.EqualError(t, err, "couldn't find factory for '*wrappers.StringValue'")

	factory.encoder = fake.BadUnmarshalDynEncoder{}
	_, err = factory.FromProto(pbany)
	require.EqualError(t, err, "couldn't decode message: fake error")
}

func TestPublicKeyFactory_VisitJSON(t *testing.T) {
	factory := NewPublicKeyFactory()
	factory.RegisterAlgorithm("fake", fake.PublicKeyFactory{})

	ser := json.NewSerializer()

	var pubkey crypto.PublicKey
	err := ser.Deserialize([]byte(`{"Name": "fake","Data":[]}`), factory, &pubkey)
	require.NoError(t, err)
	require.IsType(t, fake.PublicKey{}, pubkey)

	err = ser.Deserialize([]byte(`{"Name": "unknown"}`), factory, &pubkey)
	require.EqualError(t, xerrors.Unwrap(err), "unknown algorithm 'unknown'")

	_, err = factory.VisitJSON(fake.NewBadFactoryInput())
	require.EqualError(t, err, "couldn't deserialize algorithm: fake error")
}

func TestSignatureFactory_Register(t *testing.T) {
	factory := NewSignatureFactory()

	length := len(factory.deprecated)

	factory.Register(&empty.Empty{}, fake.SignatureFactory{})
	require.Len(t, factory.deprecated, length+1)

	factory.Register(&empty.Empty{}, fake.SignatureFactory{})
	require.Len(t, factory.deprecated, length+1)
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
	require.EqualError(t, err, "couldn't find factory for '*wrappers.BoolValue'")

	factory.encoder = fake.BadUnmarshalDynEncoder{}
	_, err = factory.FromProto(sigany)
	require.EqualError(t, err, "couldn't decode message: fake error")
}

func TestSignatureFactory_VisitJSON(t *testing.T) {
	factory := NewSignatureFactory()
	factory.RegisterAlgorithm("fake", fake.SignatureFactory{})

	ser := json.NewSerializer()

	var sig crypto.Signature
	err := ser.Deserialize([]byte(`{"Name": "fake","Data":[]}`), factory, &sig)
	require.NoError(t, err)
	require.IsType(t, fake.Signature{}, sig)

	err = ser.Deserialize([]byte(`{"Name": "unknown"}`), factory, &sig)
	require.EqualError(t, xerrors.Unwrap(err), "unknown algorithm 'unknown'")

	_, err = factory.VisitJSON(fake.NewBadFactoryInput())
	require.EqualError(t, err, "couldn't deserialize algorithm: fake error")
}
