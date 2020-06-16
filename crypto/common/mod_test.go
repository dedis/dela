package common

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde/json"
	"golang.org/x/xerrors"
)

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
