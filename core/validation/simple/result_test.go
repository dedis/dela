package simple

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
)

func init() {
	RegisterResultFormat(fake.GoodFormat, fake.Format{Msg: Result{}})
	RegisterResultFormat(fake.BadFormat, fake.NewBadFormat())
	RegisterResultFormat(serde.Format("BAD_TYPE"), fake.Format{Msg: fake.Message{}})
	RegisterTransactionResultFormat(fake.GoodFormat, fake.Format{Msg: TransactionResult{}})
	RegisterTransactionResultFormat(fake.BadFormat, fake.NewBadFormat())
}

func TestTransactionResult_GetTransaction(t *testing.T) {
	res := NewTransactionResult(fakeTx{}, true, "")
	require.Equal(t, fakeTx{}, res.GetTransaction())
}

func TestTransactionResult_GetStatus(t *testing.T) {
	res := NewTransactionResult(fakeTx{}, true, "")

	accepted, reason := res.GetStatus()
	require.True(t, accepted)
	require.Equal(t, "", reason)
}

func TestTransactionResult_Serialize(t *testing.T) {
	res := NewTransactionResult(fakeTx{}, true, "")

	data, err := res.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, fake.GetFakeFormatValue(), data)

	_, err = res.Serialize(fake.NewBadContext())
	require.EqualError(t, err, fake.Err("encoding failed"))
}

func TestTransactionResultFactory_Deserialize(t *testing.T) {
	fac := NewTransactionResultFactory(nil)

	msg, err := fac.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, TransactionResult{}, msg)

	_, err = fac.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, fake.Err("decoding failed"))
}

func TestResult_GetTransactionResults(t *testing.T) {
	res := Result{
		txs: []TransactionResult{{}, {}},
	}

	require.Len(t, res.GetTransactionResults(), 2)
}

func TestResult_Fingerprint(t *testing.T) {
	res := Result{
		txs: []TransactionResult{
			{tx: fakeTx{}},
			{tx: fakeTx{}, accepted: true},
		},
	}

	buffer := new(bytes.Buffer)
	err := res.Fingerprint(buffer)
	require.NoError(t, err)

	err = res.Fingerprint(fake.NewBadHash())
	require.EqualError(t, err, fake.Err("couldn't write accepted"))

	res.txs[0].tx = fakeTx{err: fake.GetError()}
	err = res.Fingerprint(buffer)
	require.EqualError(t, err, fake.Err("couldn't fingerprint tx"))
}

func TestResult_Serialize(t *testing.T) {
	res := NewResult(nil)

	data, err := res.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, fake.GetFakeFormatValue(), data)

	_, err = res.Serialize(fake.NewBadContext())
	require.EqualError(t, err, fake.Err("encoding failed"))
}

func TestResultFactory_Deserialize(t *testing.T) {
	fac := NewResultFactory(nil)

	msg, err := fac.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, Result{}, msg)

	_, err = fac.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, fake.Err("decoding failed"))

	_, err = fac.Deserialize(fake.NewContextWithFormat(serde.Format("BAD_TYPE")), nil)
	require.EqualError(t, err, "invalid result type 'fake.Message'")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeTx struct {
	txn.Transaction

	nonce  uint64
	pubkey crypto.PublicKey
	err    error
}

func newTx() fakeTx {
	return fakeTx{
		pubkey: fake.PublicKey{},
	}
}

func (tx fakeTx) GetID() []byte {
	return []byte{0xa, 0xb, 0xc, 0xd}
}

func (tx fakeTx) GetIdentity() access.Identity {
	return tx.pubkey
}

func (tx fakeTx) GetNonce() uint64 {
	return tx.nonce
}

func (tx fakeTx) Fingerprint(io.Writer) error {
	return tx.err
}
