package native

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestService_Execute(t *testing.T) {
	srvc := NewExecution()
	srvc.Set("abc", fakeExec{})
	srvc.Set("bad", fakeExec{err: fake.GetError()})

	step := execution.Step{}
	step.Current = fakeTx{contract: "abc"}

	res, err := srvc.Execute(nil, step)
	require.NoError(t, err)
	require.Equal(t, execution.Result{Accepted: true}, res)

	step.Current = fakeTx{contract: "bad"}
	res, err = srvc.Execute(nil, step)
	require.NoError(t, err)
	require.Equal(t, execution.Result{Message: fake.GetError().Error()}, res)

	step.Current = fakeTx{contract: "none"}
	_, err = srvc.Execute(nil, step)
	require.EqualError(t, err, "unknown contract 'none'")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeExec struct {
	err error
}

func (e fakeExec) Execute(store.Snapshot, execution.Step) error {
	return e.err
}

type fakeTx struct {
	txn.Transaction
	contract string
}

func (tx fakeTx) GetArg(key string) []byte {
	return []byte(tx.contract)
}
