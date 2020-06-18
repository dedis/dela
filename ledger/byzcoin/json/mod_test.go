package json

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/ledger/byzcoin/types"
	"go.dedis.ch/dela/ledger/transactions"
	"go.dedis.ch/dela/serdeng"
	"golang.org/x/xerrors"
)

func TestBlueprint_VisitJSON(t *testing.T) {
	blueprint := types.NewBlueprint(makeTxs(nil))

	format := messageFormat{}
	ctx := serdeng.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, blueprint)
	require.NoError(t, err)
	expected := `{"Blueprint":{"Transactions":[{}]}}`
	require.Equal(t, expected, string(data))

	blueprint = types.NewBlueprint(makeTxs(xerrors.New("oops")))
	_, err = format.Encode(ctx, blueprint)
	require.EqualError(t, err, "couldn't serialize tx: oops")
}

func TestGenesisPayload_VisitJSON(t *testing.T) {
	payload := types.NewGenesisPayload([]byte{2}, fakeAuthority{})

	format := messageFormat{}
	ctx := serdeng.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, payload)
	require.NoError(t, err)
	expected := `{"GenesisPayload":{"Roster":{},"Root":"Ag=="}}`
	require.Regexp(t, expected, string(data))

	payload = types.NewGenesisPayload(nil, fakeAuthority{err: xerrors.New("oops")})
	_, err = format.Encode(ctx, payload)
	require.EqualError(t, err, "couldn't serialize roster: oops")
}

func TestBlockPayload_VisitJSON(t *testing.T) {
	payload := types.NewBlockPayload([]byte{4}, makeTxs(nil))

	format := messageFormat{}
	ctx := serdeng.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, payload)
	require.NoError(t, err)
	expected := `{"BlockPayload":{"Transactions":[{}],"Root":"BA=="}}`
	require.Equal(t, expected, string(data))

	payload = types.NewBlockPayload(nil, makeTxs(xerrors.New("oops")))
	_, err = format.Encode(ctx, payload)
	require.EqualError(t, err, "couldn't serialize tx: oops")
}

func TestMessageFactory_VisitJSON(t *testing.T) {
	format := messageFormat{}
	ctx := serdeng.NewContext(fake.ContextEngine{})
	ctx = serdeng.WithFactory(ctx, types.RosterKey{}, fakeAuthorityFactory{})
	ctx = serdeng.WithFactory(ctx, types.TxKey{}, fakeTxFactory{})

	// Blueprint message.
	blueprint, err := format.Decode(ctx, []byte(`{"Blueprint":{"Transactions":[{}]}}`))
	require.NoError(t, err)
	require.Equal(t, types.NewBlueprint(makeTxs(nil)), blueprint)

	badCtx := serdeng.WithFactory(ctx, types.TxKey{}, fakeTxFactory{err: xerrors.New("oops")})
	_, err = format.Decode(badCtx, []byte(`{"Blueprint":{"Transactions":[{}]}}`))
	require.EqualError(t, err,
		"couldn't deserialize blueprint: couldn't deserialize tx: oops")

	// BlockPayload message.
	payload, err := format.Decode(ctx, []byte(`{"BlockPayload":{"Root":[1],"Transactions":[{}]}}`))
	require.NoError(t, err)
	require.Equal(t, types.NewBlockPayload([]byte{1}, makeTxs(nil)), payload)

	_, err = format.Decode(badCtx, []byte(`{"BlockPayload":{"Transactions":[{}]}}`))
	require.EqualError(t, err,
		"couldn't deserialize payload: couldn't deserialize tx: oops")

	// GenesisPayload message.
	genesis, err := format.Decode(ctx, []byte(`{"GenesisPayload":{"Roster":{},"Root":[2]}}`))
	require.NoError(t, err)
	require.Equal(t, types.NewGenesisPayload([]byte{2}, fakeAuthority{}), genesis)

	badCtx = serdeng.WithFactory(ctx, types.RosterKey{}, fakeAuthorityFactory{err: xerrors.New("oops")})
	_, err = format.Decode(badCtx, []byte(`{"GenesisPayload":{"Roster":[]}}`))
	require.EqualError(t, err, "couldn't deserialize roster: oops")

	// Common.
	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, "couldn't deserialize message: fake error")

	_, err = format.Decode(ctx, []byte(`{}`))
	require.EqualError(t, err, "message is empty")
}

// -----------------------------------------------------------------------------
// Utility functions

func makeTxs(err error) []transactions.ServerTransaction {
	return []transactions.ServerTransaction{
		fakeTx{err: err},
	}
}

type fakeTx struct {
	transactions.ServerTransaction
	err error
}

func (tx fakeTx) Serialize(serdeng.Context) ([]byte, error) {
	return []byte(`{}`), tx.err
}

type fakeAuthority struct {
	viewchange.Authority

	err error
}

func (a fakeAuthority) Serialize(serdeng.Context) ([]byte, error) {
	return []byte(`{}`), a.err
}

type fakeAuthorityFactory struct {
	viewchange.AuthorityFactory

	err error
}

func (f fakeAuthorityFactory) AuthorityOf(serdeng.Context, []byte) (viewchange.Authority, error) {
	return fakeAuthority{}, f.err
}

type fakeTxFactory struct {
	transactions.TxFactory
	err error
}

func (f fakeTxFactory) TxOf(serdeng.Context, []byte) (transactions.ServerTransaction, error) {
	return fakeTx{}, f.err
}
