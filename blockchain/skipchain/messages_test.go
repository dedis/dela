package skipchain

import (
	"testing"

	"github.com/stretchr/testify/require"
	types "go.dedis.ch/dela/blockchain/skipchain/json"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde/json"
)

func TestBlueprint_VisitJSON(t *testing.T) {
	blueprint := Blueprint{
		index:    5,
		previous: Digest{1, 2, 3},
		data:     fake.Message{},
	}

	ser := json.NewSerializer()

	data, err := ser.Serialize(blueprint)
	require.NoError(t, err)
	require.Regexp(t, `{"Index":5,"Previous":"[^"]+","Payload":"[^"]+"}`, string(data))

	_, err = blueprint.VisitJSON(fake.NewBadSerializer())
	require.EqualError(t, err, "couldn't serialize payload: fake error")
}

func TestBlueprintFactory_VisitJSON(t *testing.T) {
	factory := BlueprintFactory{
		factory: fake.MessageFactory{},
	}

	ser := json.NewSerializer()
	data, err := ser.Serialize(Blueprint{index: 1, previous: Digest{}, data: fake.Message{}})
	require.NoError(t, err)

	var blueprint Blueprint
	err = ser.Deserialize(data, factory, &blueprint)
	require.NoError(t, err)
	require.Equal(t, uint64(1), blueprint.index)

	_, err = factory.VisitJSON(fake.NewBadFactoryInput())
	require.EqualError(t, err, "couldn't deserialize blueprint: fake error")

	_, err = factory.VisitJSON(fake.FactoryInput{Serde: fake.NewBadSerializer()})
	require.EqualError(t, err, "couldn't deserialize payload: fake error")
}

func TestPropagateGenesis_VisitJSON(t *testing.T) {
	p := PropagateGenesis{
		genesis: SkipBlock{Payload: fake.Message{}},
	}

	ser := json.NewSerializer()

	data, err := ser.Serialize(p)
	require.NoError(t, err)
	expected := `{"Genesis":{"Index":0,"GenesisID":"[^"]+","Backlink":"[^"]+","Payload":{}}}`
	require.Regexp(t, expected, string(data))

	_, err = p.VisitJSON(fake.NewBadSerializer())
	require.EqualError(t, err, "couldn't serialize genesis: fake error")
}

func TestBlockRequest_VisitJSON(t *testing.T) {
	req := BlockRequest{from: 1, to: 5}

	ser := json.NewSerializer()

	data, err := ser.Serialize(req)
	require.NoError(t, err)
	require.Equal(t, `{"Request":{"From":1,"To":5}}`, string(data))
}

func TestBlockResponse_VisitJSON(t *testing.T) {
	resp := BlockResponse{
		block: SkipBlock{Payload: fake.Message{}},
	}

	ser := json.NewSerializer()

	data, err := ser.Serialize(resp)
	require.NoError(t, err)
	expected := `{"Block":{"Index":0,"GenesisID":"[^"]+","Backlink":"[^"]+","Payload":{}}}`
	require.Regexp(t, expected, string(data))

	_, err = resp.VisitJSON(fake.NewBadSerializer())
	require.EqualError(t, err, "couldn't serialize block: fake error")
}

func TestMessageFactory_VisitJSON(t *testing.T) {
	factory := MessageFactory{
		blockFactory: NewBlockFactory(fake.MessageFactory{}),
	}

	ser := json.NewSerializer()

	var msg PropagateGenesis
	err := ser.Deserialize([]byte(`{"Propagate":{"Genesis":{}}}`), factory, &msg)
	require.NoError(t, err)

	input := fake.FactoryInput{
		Serde:   fake.NewBadSerializer(),
		Message: types.Message{Propagate: &types.PropagateGenesis{}},
	}

	_, err = factory.VisitJSON(input)
	require.EqualError(t, err, "couldn't deserialize genesis: fake error")

	var req BlockRequest
	err = ser.Deserialize([]byte(`{"Request":{}}`), factory, &req)
	require.NoError(t, err)

	var resp BlockResponse
	err = ser.Deserialize([]byte(`{"Response":{"Block":{}}}`), factory, &resp)
	require.NoError(t, err)

	input.Message = types.Message{Response: &types.BlockResponse{}}
	_, err = factory.VisitJSON(input)
	require.EqualError(t, err, "couldn't deserialize block: fake error")

	_, err = factory.VisitJSON(fake.NewBadFactoryInput())
	require.EqualError(t, err, "couldn't deserialize message: fake error")
}
