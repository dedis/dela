package common

import (
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
)

const testAlgorithm = "fake"

func init() {
	RegisterAlgorithmFormat(fake.GoodFormat, fake.Format{
		Msg: Algorithm{name: testAlgorithm},
	})
	RegisterAlgorithmFormat(serde.Format("BAD_TYPE"), fake.Format{Msg: fake.Message{}})
	RegisterAlgorithmFormat(fake.BadFormat, fake.NewBadFormat())
}

func TestAlgorithm_GetName(t *testing.T) {
	f := func(name string) bool {
		algo := NewAlgorithm(name)

		return name == algo.GetName()
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestAlgorithm_Serialize(t *testing.T) {
	algo := NewAlgorithm(testAlgorithm)

	data, err := algo.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, fake.GetFakeFormatValue(), data)

	_, err = algo.Serialize(fake.NewBadContext())
	require.EqualError(t, err, fake.Err("couldn't encode algorithm"))
}

func TestPublicKeyFactory_RegisterAlgorithm(t *testing.T) {
	factory := NewPublicKeyFactory()

	// Check passive registrations.
	require.Len(t, factory.factories, 1)

	factory.RegisterAlgorithm(testAlgorithm, fake.PublicKeyFactory{})
	require.Len(t, factory.factories, 2)
}

func TestPublicKeyFactory_Deserialize(t *testing.T) {
	factory := NewPublicKeyFactory()
	factory.RegisterAlgorithm(testAlgorithm, fake.PublicKeyFactory{})

	msg, err := factory.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, fake.PublicKey{}, msg)

	_, err = factory.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, fake.Err("couldn't decode algorithm"))

	_, err = factory.Deserialize(fake.NewContextWithFormat(serde.Format("BAD_TYPE")), nil)
	require.EqualError(t, err, "invalid message of type 'fake.Message'")

	factory = NewPublicKeyFactory()
	_, err = factory.Deserialize(fake.NewContext(), nil)
	require.EqualError(t, err, "unknown algorithm 'fake'")
}

func TestPublicKeyFactory_PublicKeyOf(t *testing.T) {
	factory := NewPublicKeyFactory()
	factory.RegisterAlgorithm(testAlgorithm, fake.PublicKeyFactory{})

	pk, err := factory.PublicKeyOf(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, fake.PublicKey{}, pk)

	_, err = factory.PublicKeyOf(fake.NewBadContext(), nil)
	require.EqualError(t, err, fake.Err("couldn't decode algorithm"))
}

func TestSignatureFactory_RegisterAlgorithm(t *testing.T) {
	factory := NewSignatureFactory()

	require.Len(t, factory.factories, 1)

	factory.RegisterAlgorithm("fake", fake.SignatureFactory{})
	require.Len(t, factory.factories, 2)
}

func TestSignatureFactory_Deserialize(t *testing.T) {
	factory := NewSignatureFactory()
	factory.RegisterAlgorithm(testAlgorithm, fake.SignatureFactory{})

	msg, err := factory.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, fake.Signature{}, msg)

	_, err = factory.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, fake.Err("couldn't decode algorithm"))

	_, err = factory.Deserialize(fake.NewContextWithFormat(serde.Format("BAD_TYPE")), nil)
	require.EqualError(t, err, "invalid message of type 'fake.Message'")

	factory = NewSignatureFactory()
	_, err = factory.Deserialize(fake.NewContext(), nil)
	require.EqualError(t, err, "unknown algorithm 'fake'")
}

func TestSignatureFactory_SignatureOf(t *testing.T) {
	factory := NewSignatureFactory()
	factory.RegisterAlgorithm(testAlgorithm, fake.SignatureFactory{})

	sig, err := factory.SignatureOf(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, fake.Signature{}, sig)

	_, err = factory.SignatureOf(fake.NewBadContext(), nil)
	require.EqualError(t, err, fake.Err("couldn't decode algorithm"))
}
