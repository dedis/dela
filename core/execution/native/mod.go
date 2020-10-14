// Package native implements an execution service to run native smart contracts.
//
// A native smart contract is written in Go and packaged with the application.
//
// Documentation Last Review: 08.10.2020
//
package native

import (
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/store"
	"golang.org/x/xerrors"
)

const (
	// ContractArg is the argument key in the transaction to look up a contract.
	ContractArg = "go.dedis.ch/dela.ContractArg"
)

// Contract is the interface to implement to register a smart contract that will
// be executed natively.
type Contract interface {
	Execute(store.Snapshot, execution.Step) error
}

// Service is an execution service for packaged applications. Those
// applications have complete access to the trie and can directly update it.
//
// - implements execution.Service
type Service struct {
	contracts map[string]Contract
}

// NewExecution returns a new native execution. The given service will be
// executed for every incoming transaction.
func NewExecution() *Service {
	return &Service{
		contracts: map[string]Contract{},
	}
}

// Set stores the contract using the name as the key. A transaction can trigger
// this contract by using the same name as the contract argument.
func (ns *Service) Set(name string, contract Contract) {
	ns.contracts[name] = contract
}

// Execute implements execution.Service. It uses the executor to process the
// incoming transaction and return the result.
func (ns *Service) Execute(snap store.Snapshot, step execution.Step) (execution.Result, error) {
	name := string(step.Current.GetArg(ContractArg))

	contract := ns.contracts[name]
	if contract == nil {
		return execution.Result{}, xerrors.Errorf("unknown contract '%s'", name)
	}

	res := execution.Result{
		Accepted: true,
	}

	err := contract.Execute(snap, step)
	if err != nil {
		res.Accepted = false
		res.Message = err.Error()
	}

	return res, nil
}
