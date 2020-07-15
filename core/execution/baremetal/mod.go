package baremetal

import (
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/tap"
	"golang.org/x/xerrors"
)

// BareMetal is an execution service for packaged applications. Those
// applications have complete access to the trie and can directly update it.
//
// - implements execution.Service
//
// TODO: extend to allow registration of other specialized bare-metal services.
type BareMetal struct {
	exec execution.Service
}

// NewExecution returns a new bare-metal execution. The given service will be
// executed for every incoming transaction.
func NewExecution(exec execution.Service) BareMetal {
	return BareMetal{
		exec: exec,
	}
}

// Execute implements execution.Service. It uses the executor to process the
// incoming transaction and return the result.
func (bm BareMetal) Execute(tx tap.Transaction, snap store.Snapshot) (execution.Result, error) {
	res, err := bm.exec.Execute(tx, snap)
	if err != nil {
		return res, xerrors.Errorf("failed to execute: %v", err)
	}

	return res, nil
}
