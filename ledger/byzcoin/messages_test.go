package byzcoin

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	"go.dedis.ch/dela/internal/testing/fake"
	types "go.dedis.ch/dela/ledger/byzcoin/json"
	"go.dedis.ch/dela/ledger/transactions"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
)

func TestBlueprint_VisitJSON(t *testing.T) {
	blueprint := Blueprint{
		transactions: []transactions.ServerTransaction{
			fakeTx{},
		},
	}

	ser := json.NewSerializer()

	data, err := ser.Serialize(blueprint)
	require.NoError(t, err)
	expected := `{"Blueprint":{"Transactions":[{}]},"GenesisPayload":null,"BlockPayload":null}`
	require.Equal(t, expected, string(data))

	_, err = blueprint.VisitJSON(fake.NewBadSerializer())
	require.EqualError(t, err, "couldn't serialize tx: fake error")
}

func TestGenesisPayload_Fingerprint(t *testing.T) {
	payload := GenesisPayload{root: []byte{5}}

	out := new(bytes.Buffer)
	err := payload.Fingerprint(out)
	require.NoError(t, err)
	require.Equal(t, "\x05", out.String())

	err = payload.Fingerprint(fake.NewBadHash())
	require.EqualError(t, err, "couldn't write root: fake error")
}

func TestGenesisPayload_VisitJSON(t *testing.T) {
	payload := GenesisPayload{
		roster: roster.New(fake.NewAuthority(1, fake.NewSigner)),
		root:   []byte{2},
	}

	ser := json.NewSerializer()

	data, err := ser.Serialize(payload)
	require.NoError(t, err)
	expected := `{"Blueprint":null,"GenesisPayload":{"Roster":\[{"Address":` +
		`"[^"]+","PublicKey":{}}\],"Root":"Ag=="},"BlockPayload":null}`
	require.Regexp(t, expected, string(data))

	_, err = payload.VisitJSON(fake.NewBadSerializer())
	require.EqualError(t, err, "couldn't serialize roster: fake error")
}

func TestBlockPayload_Fingerprint(t *testing.T) {
	payload := BlockPayload{
		root: []byte{6},
	}

	out := new(bytes.Buffer)
	err := payload.Fingerprint(out)
	require.NoError(t, err)
	require.Equal(t, "\x06", out.String())

	err = payload.Fingerprint(fake.NewBadHash())
	require.EqualError(t, err, "couldn't write root: fake error")
}

func TestBlockPayload_VisitJSON(t *testing.T) {
	payload := BlockPayload{
		transactions: []transactions.ServerTransaction{
			fakeTx{},
		},
		root: []byte{4},
	}

	ser := json.NewSerializer()

	data, err := ser.Serialize(payload)
	require.NoError(t, err)
	expected := `{"Blueprint":null,"GenesisPayload":null,` +
		`"BlockPayload":{"Transactions":[{}],"Root":"BA=="}}`
	require.Equal(t, expected, string(data))

	_, err = payload.VisitJSON(fake.NewBadSerializer())
	require.EqualError(t, err, "couldn't serialize tx: fake error")
}

func TestMessageFactory_VisitJSON(t *testing.T) {
	factory := MessageFactory{
		txFactory:     fakeTxFactory{},
		rosterFactory: roster.NewRosterFactory(fake.AddressFactory{}, fake.PublicKeyFactory{}),
	}

	ser := json.NewSerializer()

	// Blueprint message.
	var blueprint Blueprint
	err := ser.Deserialize([]byte(`{"Blueprint":{}}`), factory, &blueprint)
	require.NoError(t, err)

	input := fake.FactoryInput{
		Serde:   fake.NewBadSerializer(),
		Message: types.Message{Blueprint: &types.Blueprint{Transactions: types.Transactions{{}}}},
	}
	_, err = factory.VisitJSON(input)
	require.EqualError(t, err,
		"couldn't deserialize blueprint: couldn't deserialize tx: fake error")

	// BlockPayload message.
	var payload BlockPayload
	err = ser.Deserialize([]byte(`{"BlockPayload":{}}`), factory, &payload)
	require.NoError(t, err)

	input.Message = types.Message{
		BlockPayload: &types.BlockPayload{Transactions: types.Transactions{{}}},
	}
	_, err = factory.VisitJSON(input)
	require.EqualError(t, err,
		"couldn't deserialize payload: couldn't deserialize tx: fake error")

	// GenesisPayload message.
	var genesis GenesisPayload
	err = ser.Deserialize([]byte(`{"GenesisPayload":{"Roster":[]}}`), factory, &genesis)
	require.NoError(t, err)

	input.Message = types.Message{GenesisPayload: &types.GenesisPayload{}}
	_, err = factory.VisitJSON(input)
	require.EqualError(t, err, "couldn't deserialize roster: fake error")

	// Common.
	_, err = factory.VisitJSON(fake.NewBadFactoryInput())
	require.EqualError(t, err, "couldn't deserialize message: fake error")

	input.Message = types.Message{}
	_, err = factory.VisitJSON(input)
	require.EqualError(t, err, "message is empty")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeTxFactory struct {
	serde.UnimplementedFactory
}

func (f fakeTxFactory) VisitJSON(serde.FactoryInput) (serde.Message, error) {
	return fakeTx{}, nil
}
