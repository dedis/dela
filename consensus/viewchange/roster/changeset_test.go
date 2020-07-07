package roster

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
)

func init() {
	RegisterChangeSetFormat(fake.GoodFormat, fake.Format{Msg: ChangeSet{}})
	RegisterChangeSetFormat(serde.Format("BAD_TYPE"), fake.Format{Msg: fake.Message{}})
	RegisterChangeSetFormat(fake.BadFormat, fake.NewBadFormat())
}

func TestChangeSet_Serialize(t *testing.T) {
	cset := ChangeSet{}

	data, err := cset.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = cset.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "couldn't encode change set: fake error")
}

func TestChangeSetFactory_Deserialize(t *testing.T) {
	factory := NewChangeSetFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})

	msg, err := factory.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, ChangeSet{}, msg)

	_, err = factory.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, "couldn't decode change set: fake error")

	_, err = factory.Deserialize(fake.NewContextWithFormat(serde.Format("BAD_TYPE")), nil)
	require.EqualError(t, err, "invalid message of type 'fake.Message'")
}
