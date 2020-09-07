package viewchange

import (
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/execution/baremetal"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
	"golang.org/x/xerrors"
)

const (
	// ContractName is the name of the contract.
	ContractName = "go.dedis.ch/dela/core/execution/baremetal/viewchange.Contract"

	// AuthorityArg is the key of the argument for the new authority.
	AuthorityArg = "viewchange:authority"

	messageArgMissing       = "authority not found in transaction"
	messageStorageEmpty     = "authority not found in storage"
	messageStorageCorrupted = "invalid authority data in storage"
	messageTooManyChanges   = "too many changes"
	messageStorageFailure   = "storage failure"
)

// Manager is an extension of a normal transaction manager to help creating view
// change ones.
type Manager struct {
	manager txn.Manager
	context serde.Context
}

// NewManager returns a view change manager from the transaction manager.
func NewManager(mgr txn.Manager) Manager {
	return Manager{
		manager: mgr,
		context: json.NewContext(),
	}
}

// Make creates a new transaction using the provided manager. It contains the
// new roster that the transction should apply.
func (mgr Manager) Make(roster authority.Authority) (txn.Transaction, error) {
	data, err := roster.Serialize(mgr.context)
	if err != nil {
		return nil, xerrors.Errorf("failed to serialize roster: %v", err)
	}

	tx, err := mgr.manager.Make(
		txn.Arg{Key: baremetal.ContractArg, Value: []byte(ContractName)},
		txn.Arg{Key: AuthorityArg, Value: data},
	)
	if err != nil {
		return nil, xerrors.Errorf("creating transaction: %v", err)
	}

	return tx, nil
}

// Contract is a contract to update the roster at a given key in the storage. It
// only allows one member change per transaction.
//
// - implements baremetal.Contract
type Contract struct {
	rosterKey []byte
	rosterFac authority.Factory
	context   serde.Context
}

// NewContract creates a new viewchange contract.
func NewContract(key []byte, fac authority.Factory) Contract {
	return Contract{
		rosterKey: key,
		rosterFac: fac,
		context:   json.NewContext(),
	}
}

// Execute implements baremetal.Contract. It looks for the roster in the
// transaction and updates the storage if there is at most one membership
// change.
func (c Contract) Execute(tx txn.Transaction, snap store.Snapshot) (execution.Result, error) {
	res := execution.Result{}

	roster, err := c.rosterFac.AuthorityOf(c.context, tx.GetArg(AuthorityArg))
	if err != nil {
		res.Message = messageArgMissing
		return res, xerrors.Errorf("failed to decode arg: %v", err)
	}

	currData, err := snap.Get(c.rosterKey)
	if err != nil {
		res.Message = messageStorageEmpty
		return res, xerrors.Errorf("failed to read roster: %v", err)
	}

	curr, err := c.rosterFac.AuthorityOf(c.context, currData)
	if err != nil {
		res.Message = messageStorageCorrupted
		return res, xerrors.Errorf("failed to decode roster: %v", err)
	}

	changeset := curr.Diff(roster)

	if changeset.NumChanges() > 1 {
		res.Message = messageTooManyChanges
		return res, xerrors.Errorf("only one change is expected but found %d", changeset.NumChanges())
	}

	// TODO: check the number of changes per batch instead of transaction wide.
	// TODO: access rights control

	err = snap.Set(c.rosterKey, tx.GetArg(AuthorityArg))
	if err != nil {
		res.Message = messageStorageFailure
		return res, xerrors.Errorf("failed to store roster: %v", err)
	}

	res.Accepted = true

	return res, nil
}
