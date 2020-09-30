package authority

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
)

func init() {
	RegisterChangeSetFormat(fake.GoodFormat, fake.Format{Msg: NewChangeSet()})
	RegisterChangeSetFormat(serde.Format("BAD_TYPE"), fake.Format{Msg: fake.Message{}})
	RegisterChangeSetFormat(fake.BadFormat, fake.NewBadFormat())
}

func TestChangeSet_GetPublicKeys(t *testing.T) {
	cset := NewChangeSet()
	require.Len(t, cset.GetPublicKeys(), 0)

	cset.Add(fake.NewAddress(0), fake.PublicKey{})
	require.Len(t, cset.GetPublicKeys(), 1)
}

func TestChangeSet_GetNewAddresses(t *testing.T) {
	cset := NewChangeSet()
	require.Len(t, cset.GetNewAddresses(), 0)

	cset.Add(fake.NewAddress(0), fake.PublicKey{})
	require.Len(t, cset.GetNewAddresses(), 1)
}

func TestChangeSet_GetRemoveIndices(t *testing.T) {
	cset := NewChangeSet()
	require.Len(t, cset.GetRemoveIndices(), 0)

	cset.Remove(5)
	require.Len(t, cset.GetRemoveIndices(), 1)
}

func TestChangeSet_NumChanges(t *testing.T) {
	cset := NewChangeSet()
	require.Equal(t, 0, cset.NumChanges())

	cset.Remove(5)
	require.Equal(t, 1, cset.NumChanges())

	cset.Add(fake.NewAddress(0), fake.PublicKey{})
	require.Equal(t, 2, cset.NumChanges())
}

func TestChangeSet_Serialize(t *testing.T) {
	cset := RosterChangeSet{}

	data, err := cset.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, fake.GetFakeFormatValue(), data)

	_, err = cset.Serialize(fake.NewBadContext())
	require.EqualError(t, err, fake.Err("couldn't encode change set"))
}

func TestChangeSetFactory_Deserialize(t *testing.T) {
	factory := NewChangeSetFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})

	msg, err := factory.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, NewChangeSet(), msg)

	_, err = factory.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, fake.Err("couldn't decode change set"))

	_, err = factory.Deserialize(fake.NewContextWithFormat(serde.Format("BAD_TYPE")), nil)
	require.EqualError(t, err, "invalid message of type 'fake.Message'")
}
