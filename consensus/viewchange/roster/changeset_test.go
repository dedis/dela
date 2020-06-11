package roster

import (
	"testing"

	"github.com/stretchr/testify/require"
	types "go.dedis.ch/dela/consensus/viewchange/roster/json"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde/json"
)

func TestChangeSet_VisitJSON(t *testing.T) {
	changeset := ChangeSet{
		Remove: []uint32{42},
		Add: []Player{
			{
				Address:   fake.NewAddress(2),
				PublicKey: fake.PublicKey{},
			},
		},
	}

	ser := json.NewSerializer()

	data, err := ser.Serialize(changeset)
	require.NoError(t, err)
	expected := `{"Remove":[42],"Add":[{"Address":"AgAAAA==","PublicKey":{}}]}`
	require.Equal(t, expected, string(data))

	_, err = changeset.VisitJSON(fake.NewBadSerializer())
	require.EqualError(t, err, "couldn't serialize public key: fake error")

	changeset.Add[0].Address = fake.NewBadAddress()
	_, err = changeset.VisitJSON(ser)
	require.EqualError(t, err, "couldn't serialize address: fake error")
}

func TestChangeSetFactory_VisitJSON(t *testing.T) {
	factory := NewChangeSetFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})

	ser := json.NewSerializer()

	var changeset ChangeSet
	err := ser.Deserialize([]byte(`{"Add":[{}]}`), factory, &changeset)
	require.NoError(t, err)
	require.Len(t, changeset.Add, 1)

	_, err = factory.VisitJSON(fake.NewBadFactoryInput())
	require.EqualError(t, err, "couldn't deserialize change set: fake error")

	input := fake.FactoryInput{
		Serde:   fake.NewBadSerializer(),
		Message: types.ChangeSet{Add: []types.Player{{}}},
	}
	_, err = factory.VisitJSON(input)
	require.EqualError(t, err, "couldn't deserialize public key: fake error")
}
