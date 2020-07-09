package bmexec

import (
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/tap"
)

// BareMetal is an execution service for packaged applications. Those
// applications have complete access to the trie and can directly update it.
type BareMetal struct{}

func NewExecution() BareMetal {
	return BareMetal{}
}

func (bm BareMetal) Execute(tx tap.Transaction, trie store.ReadWriteTrie) (execution.Result, error) {
	return execution.Result{}, nil
}
