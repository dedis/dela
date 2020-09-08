package baremetal

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/txn"
	"golang.org/x/xerrors"
)

func TestBareMetal_Execute(t *testing.T) {
	srvc := NewExecution()
	srvc.Set("abc", fakeExec{})
	srvc.Set("bad", fakeExec{err: xerrors.New("oops")})

	res, err := srvc.Execute(fakeTx{contract: "abc"}, nil)
	require.NoError(t, err)
	require.Equal(t, execution.Result{Accepted: true}, res)

	res, err = srvc.Execute(fakeTx{contract: "bad"}, nil)
	require.NoError(t, err)
	require.Equal(t, execution.Result{Message: "oops"}, res)

	_, err = srvc.Execute(fakeTx{contract: "none"}, nil)
	require.EqualError(t, err, "unknown contract 'none'")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeExec struct {
	err error
}

func (e fakeExec) Execute(txn.Transaction, store.Snapshot) error {
	return e.err
}

type fakeTx struct {
	txn.Transaction
	contract string
}

func (tx fakeTx) GetArg(key string) []byte {
	return []byte(tx.contract)
}
