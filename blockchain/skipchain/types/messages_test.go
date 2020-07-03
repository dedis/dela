package types

import (
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
)

func init() {
	RegisterBlueprintFormat(fake.GoodFormat, fake.Format{Msg: Blueprint{}})
	RegisterBlueprintFormat(fake.BadFormat, fake.NewBadFormat())
	RegisterRequestFormat(fake.GoodFormat, fake.Format{Msg: PropagateGenesis{}})
	RegisterRequestFormat(fake.BadFormat, fake.NewBadFormat())
}

func TestBlueprint_Getters(t *testing.T) {
	f := func(index uint64, prev [32]byte) bool {
		bp := NewBlueprint(index, prev[:], fake.Message{})

		require.Equal(t, prev[:], bp.GetPrevious())
		require.Equal(t, fake.Message{}, bp.GetData())

		return index == bp.GetIndex()
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestBlueprint_Serialize(t *testing.T) {
	bp := Blueprint{}

	data, err := bp.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = bp.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "couldn't encode blueprint: fake error")
}

func TestBlueprintFactory_Deserialize(t *testing.T) {
	factory := NewBlueprintFactory(fake.MessageFactory{})

	msg, err := factory.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, Blueprint{}, msg)

	_, err = factory.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, "couldn't decode blueprint: fake error")
}

func TestPropagateGenesis_GetGenesis(t *testing.T) {
	p := NewPropagateGenesis(SkipBlock{})

	require.Equal(t, SkipBlock{}, p.GetGenesis())
}

func TestPropagateGenesis_Serialize(t *testing.T) {
	p := PropagateGenesis{}

	msg, err := p.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(msg))

	_, err = p.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "couldn't encode request: fake error")
}

func TestBlockRequest_Getters(t *testing.T) {
	f := func(from, to uint64) bool {
		req := NewBlockRequest(from, to)

		return from == req.GetFrom() && to == req.GetTo()
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestBlockRequest_Serialize(t *testing.T) {
	req := BlockRequest{}

	data, err := req.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = req.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "couldn't encode request: fake error")
}

func TestBlockResponse_GetBlock(t *testing.T) {
	resp := NewBlockResponse(SkipBlock{})

	require.Equal(t, SkipBlock{}, resp.GetBlock())
}

func TestBlockResponse_Serialize(t *testing.T) {
	resp := BlockResponse{}

	data, err := resp.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = resp.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "couldn't encode response: fake error")
}

func TestRequestFactory_Deserialize(t *testing.T) {
	factory := NewRequestFactory(fake.MessageFactory{})

	msg, err := factory.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, PropagateGenesis{}, msg)

	_, err = factory.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, "couldn't decode request: fake error")
}
