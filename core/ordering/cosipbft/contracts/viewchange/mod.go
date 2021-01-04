// Package viewchange implements a native smart contract to update the roster of
// a chain.
//
// Documentation Last Review: 08.10.2020
//
package viewchange

import (
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/execution/native"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
	"golang.org/x/xerrors"
)

const (
	// ContractName is the name of the contract.
	ContractName = "go.dedis.ch/dela.ViewChange"

	// AuthorityArg is the key of the argument for the new authority.
	AuthorityArg = "viewchange:authority"

	messageOnlyOne          = "only one view change per block is allowed"
	messageArgMissing       = "authority not found in transaction"
	messageStorageEmpty     = "authority not found in storage"
	messageStorageCorrupted = "invalid authority data in storage"
	messageTooManyChanges   = "too many changes"
	messageStorageFailure   = "storage failure"
	messageDuplicate        = "duplicate in roster"
	messageUnauthorized     = "unauthorized identity"
)

// RegisterContract registers the view change contract to the given execution
// service.
func RegisterContract(exec *native.Service, c Contract) {
	exec.Set(ContractName, c)
}

// NewCreds creates new credentials for a view change contract execution.
func NewCreds(id []byte) access.Credential {
	return access.NewContractCreds(id, ContractName, "update")
}

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
// new roster that the transaction should apply.
func (mgr Manager) Make(roster authority.Authority) (txn.Transaction, error) {
	data, err := roster.Serialize(mgr.context)
	if err != nil {
		return nil, xerrors.Errorf("failed to serialize roster: %v", err)
	}

	tx, err := mgr.manager.Make(
		txn.Arg{Key: native.ContractArg, Value: []byte(ContractName)},
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
// - implements native.Contract
type Contract struct {
	rosterKey []byte
	rosterFac authority.Factory
	accessKey []byte
	access    access.Service
	context   serde.Context
}

// NewContract creates a new viewchange contract.
func NewContract(rKey, aKey []byte, rFac authority.Factory, srvc access.Service) Contract {
	return Contract{
		rosterKey: rKey,
		rosterFac: rFac,
		accessKey: aKey,
		access:    srvc,
		context:   json.NewContext(),
	}
}

// Execute implements native.Contract. It looks for the roster in the
// transaction and updates the storage if there is at most one membership
// change.
func (c Contract) Execute(snap store.Snapshot, step execution.Step) error {
	for _, tx := range step.Previous {
		// Only one view change transaction is allowed per block to prevent
		// malicious peers to reach the threshold.
		if string(tx.GetArg(native.ContractArg)) == ContractName {
			return xerrors.New(messageOnlyOne)
		}
	}

	roster, err := c.rosterFac.AuthorityOf(c.context, step.Current.GetArg(AuthorityArg))
	if err != nil {
		reportErr(step.Current, xerrors.Errorf("incoming roster: %v", err))

		return xerrors.New(messageArgMissing)
	}

	currData, err := snap.Get(c.rosterKey)
	if err != nil {
		reportErr(step.Current, xerrors.Errorf("reading store: %v", err))

		return xerrors.New(messageStorageEmpty)
	}

	curr, err := c.rosterFac.AuthorityOf(c.context, currData)
	if err != nil {
		reportErr(step.Current, xerrors.Errorf("stored roster: %v", err))

		return xerrors.New(messageStorageCorrupted)
	}

	changeset := curr.Diff(roster)

	if changeset.NumChanges() > 1 {
		return xerrors.New(messageTooManyChanges)
	}

	for _, addr := range changeset.GetNewAddresses() {
		_, index := curr.GetPublicKey(addr)
		if index >= 0 {
			return xerrors.Errorf("%s: %v", messageDuplicate, addr)
		}
	}

	creds := NewCreds(c.accessKey)

	err = c.access.Match(snap, creds, step.Current.GetIdentity())
	if err != nil {
		reportErr(step.Current, xerrors.Errorf("access control: %v", err))

		return xerrors.Errorf("%s: %v", messageUnauthorized, step.Current.GetIdentity())
	}

	err = snap.Set(c.rosterKey, step.Current.GetArg(AuthorityArg))
	if err != nil {
		reportErr(step.Current, xerrors.Errorf("writing store: %v", err))

		return xerrors.New(messageStorageFailure)
	}

	return nil
}

// reportErr prints a log with the actual error while the transaction will
// contain a simplified explanation.
func reportErr(tx txn.Transaction, err error) {
	dela.Logger.Warn().
		Hex("ID", tx.GetID()).
		Err(err).
		Msg("transaction refused")
}
