package skipchain

import (
	"testing"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde/json"
)

func TestBlueprint_VisitJSON(t *testing.T) {
	blueprint := Blueprint{
		index:    5,
		previous: Digest{1, 2, 3},
		payload:  &empty.Empty{},
	}

	ser := json.NewSerializer()

	data, err := ser.Serialize(blueprint)
	require.NoError(t, err)
	require.Regexp(t, `{"Index":5,"Previous":"[^"]+","Payload":"[^"]+"}`, string(data))
}

func TestBlueprintFactory_VisitJSON(t *testing.T) {
	factory := BlueprintFactory{}

	ser := json.NewSerializer()
	data, err := ser.Serialize(Blueprint{index: 1, previous: Digest{}, payload: &empty.Empty{}})
	require.NoError(t, err)

	var blueprint Blueprint
	err = ser.Deserialize(data, factory, &blueprint)
	require.NoError(t, err)
	require.Equal(t, uint64(1), blueprint.index)

	_, err = factory.VisitJSON(fake.NewBadFactoryInput())
	require.EqualError(t, err, "couldn't deserialize blueprint: fake error")
}
