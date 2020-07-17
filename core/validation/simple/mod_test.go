package simple

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/tap"
	"go.dedis.ch/dela/core/tap/anon"
	"golang.org/x/xerrors"
)

func TestService_Validate(t *testing.T) {
	srvc := NewService(fakeExec{})

	data, err := srvc.Validate(fakeSnapshot{}, []tap.Transaction{makeTx(t)})
	require.NoError(t, err)
	require.NotNil(t, data)

	srvc.execution = fakeExec{err: xerrors.New("oops")}
	_, err = srvc.Validate(fakeSnapshot{}, []tap.Transaction{makeTx(t)})
	require.EqualError(t, err, "failed to execute tx: oops")
}

// -----------------------------------------------------------------------------
// Utility functions

func makeTx(t *testing.T) tap.Transaction {
	tx, err := anon.NewTransaction(0)
	require.NoError(t, err)
	return tx
}

type fakeExec struct {
	err error
}

func (e fakeExec) Execute(tap.Transaction, store.Snapshot) (execution.Result, error) {
	return execution.Result{}, e.err
}

type fakeSnapshot struct {
	store.Snapshot
}
