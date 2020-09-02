package json

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/anon"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestTxFormat_Encode(t *testing.T) {
	format := txFormat{}

	ctx := fake.NewContext()

	data, err := format.Encode(ctx, makeTx(t, 1, anon.WithArg("A", []byte{1})))
	require.NoError(t, err)
	require.Equal(t, `{"Nonce":1,"Args":{"A":"AQ=="}}`, string(data))

	_, err = format.Encode(ctx, fake.Message{})
	require.EqualError(t, err, "unsupported message of type 'fake.Message'")

	_, err = format.Encode(fake.NewBadContext(), makeTx(t, 0))
	require.EqualError(t, err, "failed to marshal: fake error")
}

func TestTxFormat_Decode(t *testing.T) {
	format := txFormat{}

	ctx := fake.NewContext()

	msg, err := format.Decode(ctx, []byte(`{"Nonce":2,"Args":{"B":"AQ=="}}`))
	require.NoError(t, err)
	require.Equal(t, makeTx(t, 2, anon.WithArg("B", []byte{1})), msg)

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, "failed to unmarshal: fake error")

	format.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	_, err = format.Decode(ctx, []byte(`{}`))
	require.EqualError(t, err,
		"failed to create tx: couldn't fingerprint tx: couldn't write nonce: fake error")
}

// -----------------------------------------------------------------------------
// Utility functions

func makeTx(t *testing.T, nonce uint64, opts ...anon.TransactionOption) txn.Transaction {
	tx, err := anon.NewTransaction(nonce, opts...)
	require.NoError(t, err)

	return tx
}
