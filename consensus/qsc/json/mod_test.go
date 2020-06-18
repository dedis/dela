package json

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/consensus/qsc/types"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serdeng"
)

func TestHistoryFormat_Encode(t *testing.T) {
	hist := types.NewHistory(types.NewEpoch([]byte{0xa}, 10))

	format := historyFormat{}
	ctx := serdeng.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, hist)
	require.NoError(t, err)
	require.Equal(t, `[{"Hash":"Cg==","Random":10}]`, string(data))
}

func TestHistoryFactory_VisitJSON(t *testing.T) {
	format := historyFormat{}
	ctx := serdeng.NewContext(fake.ContextEngine{})

	hist, err := format.Decode(ctx, []byte(`[{},{}]`))
	require.NoError(t, err)
	require.Equal(t, types.NewHistory(types.NewEpoch(nil, 0), types.NewEpoch(nil, 0)), hist)

	_, err = format.Decode(fake.NewBadContext(), []byte(`[{},{}]`))
	require.EqualError(t, err, "couldn't deserialize message: fake error")
}

func TestRequestFormat_Proposal_Encode(t *testing.T) {
	p := types.NewProposal(fake.Message{})

	format := requestFormat{}
	ctx := serdeng.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, p)
	require.NoError(t, err)
	require.Regexp(t, `{(("\w+":null|"Proposal":{"Value":{}}),?)+}`, string(data))

	_, err = format.Encode(fake.NewBadContext(), p)
	require.EqualError(t, err, "couldn't serialize value: fake error")
}

func TestRequestFormat_MessageSet_Encode(t *testing.T) {
	mset := types.NewMessageSet(4, 999, types.NewMessage(1, fake.Message{}))

	format := requestFormat{}
	ctx := serdeng.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, mset)
	require.NoError(t, err)
	regexp := `{(("\w+":null|"MessageSet":{"Node":4,"TimeStep":999,"Messages":` +
		`{"1":{"Node":1,"Value":{(("\w+":null|"Proposal":{"Value":{}}),?)+}}}}),?)+}`
	require.Regexp(t, regexp, string(data))

	_, err = format.Encode(fake.NewBadContext(), mset)
	require.EqualError(t, err,
		"couldn't serialize message: couldn't serialize value: fake error")
}

func TestRequestFormat_Decode(t *testing.T) {
	format := requestFormat{}
	ctx := serdeng.NewContext(fake.ContextEngine{})
	ctx = serdeng.WithFactory(ctx, types.MsgKey{}, fake.MessageFactory{})

	mset, err := format.Decode(ctx, []byte(`{"MessageSet":{}}`))
	require.NoError(t, err)
	require.Equal(t, types.NewMessageSet(0, 0), mset)

	_, err = format.Decode(ctx, []byte(`{"MessageSet":{"Messages":{"1":{"Value":{}}}}}`))
	require.EqualError(t, err, "couldn't deserialize value: message is empty")

	req, err := format.Decode(ctx, []byte(`{"RequestMessageSet":{}}`))
	require.NoError(t, err)
	require.Equal(t, types.NewRequestMessageSet(0, nil), req)

	prop, err := format.Decode(ctx, []byte(`{"Proposal":{}}`))
	require.NoError(t, err)
	require.Equal(t, fake.Message{}, prop)

	badCtx := serdeng.WithFactory(ctx, types.MsgKey{}, fake.NewBadMessageFactory())
	_, err = format.Decode(badCtx, []byte(`{"Proposal":{}}`))
	require.EqualError(t, err, "couldn't deserialize value: fake error")

	_, err = format.Decode(ctx, []byte(`{}`))
	require.EqualError(t, err, "message is empty")

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, "couldn't deserialize request: fake error")
}
