package baremetal

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestBareMetal_Execute(t *testing.T) {
	srvc := NewExecution()
	srvc.Set("abc", fakeExec{})
	srvc.Set("bad", fakeExec{err: fake.GetError()})

	res, err := srvc.Execute(fakeTx{contract: "abc"}, nil)
	require.NoError(t, err)
	require.Equal(t, execution.Result{Accepted: true}, res)

	res, err = srvc.Execute(fakeTx{contract: "bad"}, nil)
	require.NoError(t, err)
	require.Equal(t, execution.Result{Message: fake.GetError().Error()}, res)

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
