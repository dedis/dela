package types

import (
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
)

func init() {
	RegisterRequestFormat(fake.GoodFormat, fake.Format{Msg: Proposal{}})
	RegisterRequestFormat(fake.BadFormat, fake.NewBadFormat())
}

func TestProposal_GetValue(t *testing.T) {
	p := NewProposal(fake.Message{})

	require.Equal(t, fake.Message{}, p.GetValue())
}

func TestProposal_Serialize(t *testing.T) {
	p := Proposal{}

	data, err := p.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = p.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "couldn't encode proposal: fake error")
}

func TestMessage_Getters(t *testing.T) {
	f := func(node int64) bool {
		m := NewMessage(node, fake.Message{})

		require.Equal(t, fake.Message{}, m.GetValue())

		return m.GetNode() == node
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestMessageSet_Getters(t *testing.T) {
	f := func(node int64, step uint64) bool {
		mset := NewMessageSet(node, step, Message{node: 0}, Message{node: 1})

		require.Len(t, mset.GetMessages(), 2)

		return node == mset.GetNode() && step == mset.GetTimeStep()
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestMessageSet_Reduce(t *testing.T) {
	mset := NewMessageSet(0, 0, Message{node: 0}, Message{node: 1}, Message{node: 2})

	r := mset.Reduce([]int64{1, 2})
	require.Len(t, r.GetMessages(), 2)

	r = mset.Reduce([]int64{1})
	require.Len(t, r.GetMessages(), 1)
}

func TestMessageSet_Merge(t *testing.T) {
	mset := NewMessageSet(3, 1, Message{node: 1})
	other := NewMessageSet(0, 0, Message{node: 1}, Message{node: 2})

	m := mset.Merge(other)
	require.Equal(t, int64(3), m.GetNode())
	require.Equal(t, uint64(1), m.GetTimeStep())
	require.Len(t, m.GetMessages(), 2)
}

func TestMessageSet_Serialize(t *testing.T) {
	mset := MessageSet{}

	data, err := mset.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = mset.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "couldn't encode message set: fake error")
}

func TestRequestMessageSet_Getters(t *testing.T) {
	f := func(step uint64, nodes []int64) bool {
		req := NewRequestMessageSet(step, nodes)

		require.Equal(t, nodes, req.GetNodes())

		return step == req.GetTimeStep()
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestRequestMessageSet_Serialize(t *testing.T) {
	req := RequestMessageSet{}

	data, err := req.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = req.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "couldn't encode request: fake error")
}

func TestRequestFactory_Deserialize(t *testing.T) {
	factory := NewRequestFactory(fake.MessageFactory{})

	msg, err := factory.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, Proposal{}, msg)

	_, err = factory.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, "couldn't decode request: fake error")
}
