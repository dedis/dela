package types

import (
	"bytes"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/ledger/transactions"
	"go.dedis.ch/dela/serde"
)

var testCalls = &fake.Call{}

func init() {
	RegisterMessageFormat(fake.GoodFormat, fake.Format{Msg: Blueprint{}, Call: testCalls})
	RegisterMessageFormat(fake.BadFormat, fake.NewBadFormat())
}

func TestBlueprint_GetTransactions(t *testing.T) {
	bp := NewBlueprint(makeTxs())

	require.Len(t, bp.GetTransactions(), 2)
}

func TestBlueprint_Serialize(t *testing.T) {
	bp := NewBlueprint(makeTxs())

	data, err := bp.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = bp.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "couldn't encode blueprint: fake error")
}

func TestGenesisPayload_GetRoot(t *testing.T) {
	f := func(root []byte) bool {
		p := NewGenesisPayload(root, nil)

		return bytes.Equal(root, p.GetRoot())
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestGenesisPayload_GetRoster(t *testing.T) {
	p := NewGenesisPayload(nil, fakeAuthority{})

	require.Equal(t, fakeAuthority{}, p.GetRoster())
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

func TestGenesisPayload_Serialize(t *testing.T) {
	p := GenesisPayload{}

	data, err := p.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = p.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "couldn't encode payload: fake error")
}

func TestBlockPayload_GetRoot(t *testing.T) {
	f := func(root []byte) bool {
		p := NewBlockPayload(root, nil)

		return bytes.Equal(root, p.GetRoot())
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
}

func TestBlockPayload_GetTransactions(t *testing.T) {
	p := NewBlockPayload(nil, makeTxs())

	require.Len(t, p.GetTransactions(), 2)
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

func TestBlockPayload_Serialize(t *testing.T) {
	p := BlockPayload{}

	data, err := p.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = p.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "couldn't encode payload: fake error")
}

func TestMessageFactory_Deserialize(t *testing.T) {
	factory := NewMessageFactory(fakeAuthorityFac{}, fakeTxFac{})

	testCalls.Clear()
	msg, err := factory.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.IsType(t, Blueprint{}, msg)

	require.Equal(t, 1, testCalls.Len())
	ctx := testCalls.Get(0, 0).(serde.Context)
	require.NotNil(t, ctx.GetFactory(RosterKey{}))
	require.NotNil(t, ctx.GetFactory(TxKey{}))

	_, err = factory.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, "couldn't decode message: fake error")
}

// -----------------------------------------------------------------------------
// Utility functions

func makeTxs() []transactions.ServerTransaction {
	return []transactions.ServerTransaction{
		fakeTx{},
		fakeTx{},
	}
}

type fakeTx struct {
	transactions.ServerTransaction
}

type fakeAuthority struct {
	viewchange.Authority
}

type fakeAuthorityFac struct {
	viewchange.AuthorityFactory
}

type fakeTxFac struct {
	transactions.TxFactory
}
