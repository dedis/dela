package baremetal

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/tap"
	"golang.org/x/xerrors"
)

func TestBareMetal_Execute(t *testing.T) {
	srvc := NewExecution(fakeExec{})

	res, err := srvc.Execute(nil, nil)
	require.NoError(t, err)
	require.Equal(t, execution.Result{}, res)

	srvc.exec = fakeExec{err: xerrors.New("oops")}
	_, err = srvc.Execute(nil, nil)
	require.EqualError(t, err, "failed to execute: oops")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeExec struct {
	err error
}

func (e fakeExec) Execute(tap.Transaction, store.Snapshot) (execution.Result, error) {
	return execution.Result{}, e.err
}
