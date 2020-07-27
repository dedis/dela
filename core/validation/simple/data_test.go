package simple

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/tap"
	"go.dedis.ch/dela/internal/testing/fake"
	"golang.org/x/xerrors"
)

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

// -----------------------------------------------------------------------------
// Utility functions

type fakeTx struct {
	tap.Transaction

	err error
}

func (tx fakeTx) Fingerprint(io.Writer) error {
	return tx.err
}
