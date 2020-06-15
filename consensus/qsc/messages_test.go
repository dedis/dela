package qsc

import (
	"testing"

	"github.com/stretchr/testify/require"
	types "go.dedis.ch/dela/consensus/qsc/json"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde/json"
)

func TestProposal_VisitJSON(t *testing.T) {
	p := Proposal{
		value: fake.Message{},
	}

	ser := json.NewSerializer()

	data, err := ser.Serialize(p)
	require.NoError(t, err)
	// Proposal should be set and other fields null.
	require.Regexp(t, `{(("\w+":null|"Proposal":{"Value":{}}),?)+}`, string(data))

	_, err = p.VisitJSON(fake.NewBadSerializer())
	require.EqualError(t, err, "couldn't serialize value: fake error")
}

func TestMessage_VisitJSON(t *testing.T) {
	m := Message{
		node:  3,
		value: fake.Message{},
	}

	ser := json.NewSerializer()

	data, err := ser.Serialize(m)
	require.NoError(t, err)
	regexp := `{"Node":3,"Value":{(("\w+":null|"Proposal":{"Value":{}}),?)+}}`
	require.Regexp(t, regexp, string(data))

	_, err = m.VisitJSON(fake.NewBadSerializer())
	require.EqualError(t, err, "couldn't serialize value: fake error")
}

func TestMessageSet_VisitJSON(t *testing.T) {
	mset := MessageSet{
		node:     4,
		timeStep: 999,
		messages: map[int64]Message{1: {value: fake.Message{}}},
	}

	ser := json.NewSerializer()

	data, err := ser.Serialize(mset)
	require.NoError(t, err)
	regexp := `{(("\w+":null|"MessageSet":{"Node":4,"TimeStep":999,"Messages":` +
		`{"1":{"Node":0,"Value":{(("\w+":null|"Proposal":{"Value":{}}),?)+}}}}),?)+}`
	require.Regexp(t, regexp, string(data))

	_, err = mset.VisitJSON(fake.NewBadSerializer())
	require.EqualError(t, err,
		"couldn't serialize message: couldn't serialize value: fake error")
}

func TestRequestFactory_VisitJSON(t *testing.T) {
	factory := RequestFactory{
		mFactory: fake.MessageFactory{},
	}

	ser := json.NewSerializer()

	var mset MessageSet
	err := ser.Deserialize([]byte(`{"MessageSet":{}}`), factory, &mset)
	require.NoError(t, err)

	input := fake.FactoryInput{
		Serde: fake.NewBadSerializer(),
		Message: types.Request{
			MessageSet: &types.MessageSet{Messages: map[int64]types.Message{1: {}}},
		},
	}
	_, err = factory.VisitJSON(input)
	require.EqualError(t, err, "couldn't deserialize value: fake error")

	var req RequestMessageSet
	err = ser.Deserialize([]byte(`{"RequestMessageSet":{}}`), factory, &req)
	require.NoError(t, err)

	var prop fake.Message
	err = ser.Deserialize([]byte(`{"Proposal":{}}`), factory, &prop)
	require.NoError(t, err)

	input.Message = types.Request{Proposal: &types.Proposal{}}
	_, err = factory.VisitJSON(input)
	require.EqualError(t, err, "couldn't deserialize value: fake error")

	input.Message = types.Request{}
	_, err = factory.VisitJSON(input)
	require.EqualError(t, err, "message is empty")

	_, err = factory.VisitJSON(fake.NewBadFactoryInput())
	require.EqualError(t, err, "couldn't deserialize request: fake error")
}
