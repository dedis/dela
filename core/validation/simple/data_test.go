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
	"golang.org/x/xerrors"
)

func init() {
	RegisterDataFormat(fake.GoodFormat, fake.Format{Msg: Data{}})
	RegisterDataFormat(fake.BadFormat, fake.NewBadFormat())
	RegisterDataFormat(serde.Format("BAD_TYPE"), fake.Format{Msg: fake.Message{}})
	RegisterResultFormat(fake.GoodFormat, fake.Format{Msg: TransactionResult{}})
	RegisterResultFormat(fake.BadFormat, fake.NewBadFormat())
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
	require.Equal(t, "fake format", string(data))

	_, err = res.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "encoding failed: fake error")
}

func TestResultFactory_Deserialize(t *testing.T) {
	fac := NewResultFactory(nil)

	msg, err := fac.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, TransactionResult{}, msg)

	_, err = fac.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, "decoding failed: fake error")
}

func TestData_GetTransactionResults(t *testing.T) {
	data := Data{
		txs: []TransactionResult{{}, {}},
	}

	require.Len(t, data.GetTransactionResults(), 2)
}

func TestData_Fingerprint(t *testing.T) {
	data := Data{
		txs: []TransactionResult{
			{tx: fakeTx{}},
			{tx: fakeTx{}, accepted: true},
		},
	}

	buffer := new(bytes.Buffer)
	err := data.Fingerprint(buffer)
	require.NoError(t, err)

	err = data.Fingerprint(fake.NewBadHash())
	require.EqualError(t, err, "couldn't write accepted: fake error")

	data.txs[0].tx = fakeTx{err: xerrors.New("oops")}
	err = data.Fingerprint(buffer)
	require.EqualError(t, err, "couldn't fingerprint tx: oops")
}

func TestData_Serialize(t *testing.T) {
	vdata := NewData(nil)

	data, err := vdata.Serialize(fake.NewContext())
	require.NoError(t, err)
	require.Equal(t, "fake format", string(data))

	_, err = vdata.Serialize(fake.NewBadContext())
	require.EqualError(t, err, "encoding failed: fake error")
}

func TestDataFactory_Deserialize(t *testing.T) {
	fac := NewDataFactory(nil)

	msg, err := fac.Deserialize(fake.NewContext(), nil)
	require.NoError(t, err)
	require.Equal(t, Data{}, msg)

	_, err = fac.Deserialize(fake.NewBadContext(), nil)
	require.EqualError(t, err, "decoding failed: fake error")

	_, err = fac.Deserialize(fake.NewContextWithFormat(serde.Format("BAD_TYPE")), nil)
	require.EqualError(t, err, "invalid data type 'fake.Message'")
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
