package baremetal

import (
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/txn"
	"golang.org/x/xerrors"
)

const (
	// ContractArg is the argument key in the transaction to look up a contract.
	ContractArg = "go.dedis.ch/dela/core/execution/baremetal.ContractArg"
)

// Contract is the interface to implement to register a smart contract that will
// be executed natively.
type Contract interface {
	Execute(txn.Transaction, store.Snapshot) (execution.Result, error)
}

// BareMetal is an execution service for packaged applications. Those
// applications have complete access to the trie and can directly update it.
//
// - implements execution.Service
type BareMetal struct {
	contracts map[string]Contract
}

// NewExecution returns a new bare-metal execution. The given service will be
// executed for every incoming transaction.
func NewExecution() *BareMetal {
	return &BareMetal{
		contracts: map[string]Contract{},
	}
}

// Set stores the contract using the name as the key. A transaction can trigger
// this contract by using the same name as the contract argument.
func (bm *BareMetal) Set(name string, contract Contract) {
	bm.contracts[name] = contract
}

// Execute implements execution.Service. It uses the executor to process the
// incoming transaction and return the result.
func (bm *BareMetal) Execute(tx txn.Transaction, snap store.Snapshot) (execution.Result, error) {
	name := string(tx.GetArg(ContractArg))

	contract := bm.contracts[name]
	if contract == nil {
		return execution.Result{}, xerrors.Errorf("unknwon contract '%s'", name)
	}

	res, err := contract.Execute(tx, snap)
	if err != nil {
		return res, xerrors.Errorf("failed to execute: %v", err)
	}

	return res, nil
}
