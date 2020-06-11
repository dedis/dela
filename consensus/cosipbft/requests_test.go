package cosipbft

import (
	"testing"

	"github.com/stretchr/testify/require"
	types "go.dedis.ch/dela/consensus/cosipbft/json"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
)

func TestPrepare_VisitJSON(t *testing.T) {
	prepare := Prepare{
		message:   fake.Message{},
		signature: fake.Signature{},
		chain:     forwardLinkChain{},
	}

	ser := json.NewSerializer()

	data, err := ser.Serialize(prepare)
	require.NoError(t, err)
	expected := `{"Prepare":{"Message":{},"Signature":{},"Chain":[]},"Commit":null}`
	require.Equal(t, expected, string(data))

	_, err = prepare.VisitJSON(fake.NewBadSerializer())
	require.EqualError(t, err, "couldn't serialize message: fake error")

	_, err = prepare.VisitJSON(fake.NewBadSerializerWithDelay(1))
	require.EqualError(t, err, "couldn't serialize signature: fake error")

	_, err = prepare.VisitJSON(fake.NewBadSerializerWithDelay(2))
	require.EqualError(t, err, "couldn't serialize chain: fake error")
}

func TestCommit_VisitJSON(t *testing.T) {
	commit := Commit{
		to:      []byte{1},
		prepare: fake.Signature{},
	}

	ser := json.NewSerializer()

	data, err := ser.Serialize(commit)
	require.NoError(t, err)
	require.Equal(t, `{"Prepare":null,"Commit":{"To":"AQ==","Prepare":{}}}`, string(data))

	_, err = commit.VisitJSON(fake.NewBadSerializer())
	require.EqualError(t, err, "couldn't serialize prepare: fake error")
}

func TestRequestFactory_VisitJSON(t *testing.T) {
	factory := requestFactory{
		msgFactory:   fake.MessageFactory{},
		sigFactory:   fake.SignatureFactory{},
		cosiFactory:  fake.SignatureFactory{},
		chainFactory: chainFactory{},
	}

	ser := json.NewSerializer()

	var request serde.Message
	err := ser.Deserialize([]byte(`{"Prepare":{"Chain":[]}}`), factory, &request)
	require.NoError(t, err)

	err = ser.Deserialize([]byte(`{"Commit":{}}`), factory, &request)
	require.NoError(t, err)

	_, err = factory.VisitJSON(fake.NewBadFactoryInput())
	require.EqualError(t, err, "couldn't deserialize request: fake error")

	input := fake.FactoryInput{
		Serde:   fake.NewBadSerializer(),
		Message: types.Request{Prepare: &types.PrepareRequest{}},
	}
	_, err = factory.VisitJSON(input)
	require.EqualError(t, err, "couldn't deserialize message: fake error")

	input.Serde = fake.NewBadSerializerWithDelay(1)
	_, err = factory.VisitJSON(input)
	require.EqualError(t, err, "couldn't deserialize signature: fake error")

	input.Serde = fake.NewBadSerializerWithDelay(2)
	_, err = factory.VisitJSON(input)
	require.EqualError(t, err, "couldn't deserialize chain: fake error")

	input = fake.FactoryInput{
		Serde:   fake.NewBadSerializer(),
		Message: types.Request{Commit: &types.CommitRequest{}},
	}
	_, err = factory.VisitJSON(input)
	require.EqualError(t, err, "couldn't deserialize commit: fake error")

	input.Message = types.Request{}
	_, err = factory.VisitJSON(input)
	require.EqualError(t, err, "request is empty")
}

func TestPropoagate_VisitJSON(t *testing.T) {
	propagate := Propagate{
		to:     []byte{1},
		commit: fake.Signature{},
	}

	ser := json.NewSerializer()

	data, err := ser.Serialize(propagate)
	require.NoError(t, err)
	require.Equal(t, `{"To":"AQ==","Commit":{}}`, string(data))

	_, err = propagate.VisitJSON(fake.NewBadSerializer())
	require.EqualError(t, err, "couldn't serialize commit: fake error")
}

func TestPropagateFactory_VisitJSON(t *testing.T) {
	factory := propagateFactory{
		sigFactory: fake.SignatureFactory{},
	}

	ser := json.NewSerializer()

	var propagate Propagate
	err := ser.Deserialize([]byte(`{"To":"AQ==","Commit":{}}`), factory, &propagate)
	require.NoError(t, err)

	_, err = factory.VisitJSON(fake.NewBadFactoryInput())
	require.EqualError(t, err, "couldn't deserialize propagate: fake error")

	input := fake.FactoryInput{
		Serde:   fake.NewBadSerializer(),
		Message: types.PropagateRequest{},
	}
	_, err = factory.VisitJSON(input)
	require.EqualError(t, err, "couldn't deserialize commit: fake error")
}
