package simple

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/anon"
	"golang.org/x/xerrors"
)

func TestService_GetFactory(t *testing.T) {
	srvc := NewService(fakeExec{}, nil)
	require.NotNil(t, srvc.GetFactory())
}

func TestService_Validate(t *testing.T) {
	srvc := NewService(fakeExec{}, nil)

	data, err := srvc.Validate(fakeSnapshot{}, []txn.Transaction{makeTx(t)})
	require.NoError(t, err)
	require.NotNil(t, data)

	srvc.execution = fakeExec{err: xerrors.New("oops")}
	_, err = srvc.Validate(fakeSnapshot{}, []txn.Transaction{makeTx(t)})
	require.EqualError(t, err, "failed to execute tx: oops")
}

// -----------------------------------------------------------------------------
// Utility functions

func makeTx(t *testing.T) txn.Transaction {
	tx, err := anon.NewTransaction(0)
	require.NoError(t, err)
	return tx
}

type fakeExec struct {
	err error
}

func (e fakeExec) Execute(txn.Transaction, store.Snapshot) (execution.Result, error) {
	return execution.Result{}, e.err
}

type fakeSnapshot struct {
	store.Snapshot
}
