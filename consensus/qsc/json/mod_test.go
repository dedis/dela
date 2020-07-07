package json

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/consensus/qsc/types"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
)

func TestHistoryFormat_Encode(t *testing.T) {
	hist := types.NewHistory(types.NewEpoch([]byte{0xa}, 10))

	format := historyFormat{}
	ctx := serde.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, hist)
	require.NoError(t, err)
	require.Equal(t, `[{"Hash":"Cg==","Random":10}]`, string(data))

	_, err = format.Encode(ctx, fake.Message{})
	require.EqualError(t, err, "unsupported message of type 'fake.Message'")

	_, err = format.Encode(fake.NewBadContext(), hist)
	require.EqualError(t, err, "couldn't marshal: fake error")
}

func TestHistoryFactory_VisitJSON(t *testing.T) {
	format := historyFormat{}
	ctx := serde.NewContext(fake.ContextEngine{})

	hist, err := format.Decode(ctx, []byte(`[{},{}]`))
	require.NoError(t, err)
	require.Equal(t, types.NewHistory(types.NewEpoch(nil, 0), types.NewEpoch(nil, 0)), hist)

	_, err = format.Decode(fake.NewBadContext(), []byte(`[{},{}]`))
	require.EqualError(t, err, "couldn't deserialize message: fake error")
}

func TestRequestFormat_Proposal_Encode(t *testing.T) {
	p := types.NewProposal(fake.Message{})

	format := requestFormat{}
	ctx := serde.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, p)
	require.NoError(t, err)
	require.Regexp(t, `{(("\w+":null|"Proposal":{"Value":{}}),?)+}`, string(data))

	_, err = format.Encode(fake.NewBadContext(), p)
	require.EqualError(t, err, "couldn't serialize value: fake error")
}

func TestRequestFormat_MessageSet_Encode(t *testing.T) {
	mset := types.NewMessageSet(4, 999, types.NewMessage(1, fake.Message{}))

	format := requestFormat{}
	ctx := serde.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, mset)
	require.NoError(t, err)
	regexp := `{(("\w+":null|"MessageSet":{"Node":4,"TimeStep":999,"Messages":` +
		`{"1":{"Node":1,"Value":{(("\w+":null|"Proposal":{"Value":{}}),?)+}}}}),?)+}`
	require.Regexp(t, regexp, string(data))

	_, err = format.Encode(fake.NewBadContext(), mset)
	require.EqualError(t, err,
		"couldn't serialize message: couldn't serialize value: fake error")

	// Encode a message set that contains message sets.
	mset = types.NewMessageSet(0, 0, types.NewMessage(0, types.MessageSet{}))
	_, err = format.Encode(ctx, mset)
	require.NoError(t, err)
}

func TestRequestFormat_RequestMessageSet_Encode(t *testing.T) {
	req := types.NewRequestMessageSet(1, []int64{2, 3, 4})

	format := requestFormat{}
	ctx := fake.NewContextWithFormat(serde.FormatJSON)

	data, err := format.Encode(ctx, req)
	require.NoError(t, err)
	require.Equal(t, `{"RequestMessageSet":{"TimeStep":1,"Nodes":[2,3,4]}}`, string(data))

	_, err = format.Encode(fake.NewBadContext(), req)
	require.EqualError(t, err, "couldn't marshal: fake error")

	_, err = format.Encode(fake.NewContext(), fake.Message{})
	require.EqualError(t, err, "unsupported message of type 'fake.Message'")
}

func TestRequestFormat_Decode(t *testing.T) {
	format := requestFormat{}
	ctx := serde.NewContext(fake.ContextEngine{})
	ctx = serde.WithFactory(ctx, types.MsgKeyFac{}, fake.MessageFactory{})

	mset, err := format.Decode(ctx, []byte(`{"MessageSet":{"Messages":{"1":{"Node":1,"Value":{"Proposal":{}}}}}}`))
	require.NoError(t, err)
	expected := types.NewMessageSet(0, 0, types.NewMessage(1, fake.Message{}))
	require.Equal(t, expected, mset)

	_, err = format.Decode(ctx, []byte(`{"MessageSet":{"Messages":{"1":{"Value":{}}}}}`))
	require.EqualError(t, err, "couldn't deserialize value: message is empty")

	req, err := format.Decode(ctx, []byte(`{"RequestMessageSet":{}}`))
	require.NoError(t, err)
	require.Equal(t, types.NewRequestMessageSet(0, nil), req)

	prop, err := format.Decode(ctx, []byte(`{"Proposal":{}}`))
	require.NoError(t, err)
	require.Equal(t, fake.Message{}, prop)

	badCtx := serde.WithFactory(ctx, types.MsgKeyFac{}, fake.NewBadMessageFactory())
	_, err = format.Decode(badCtx, []byte(`{"Proposal":{}}`))
	require.EqualError(t, err, "couldn't deserialize value: fake error")

	_, err = format.Decode(ctx, []byte(`{}`))
	require.EqualError(t, err, "message is empty")

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, "couldn't deserialize request: fake error")
}
