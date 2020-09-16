package viewchange

import (
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/core/access"
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
	ContractName = "go.dedis.ch/dela.ViewChange"

	// AuthorityArg is the key of the argument for the new authority.
	AuthorityArg = "viewchange:authority"

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
func RegisterContract(exec *baremetal.BareMetal, c Contract) {
	exec.Set(ContractName, c)
}

// NewCreds creates new credentials for a view change contract execution.
func NewCreds(id []byte) access.Credentials {
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

// Execute implements baremetal.Contract. It looks for the roster in the
// transaction and updates the storage if there is at most one membership
// change.
func (c Contract) Execute(tx txn.Transaction, snap store.Snapshot) error {
	roster, err := c.rosterFac.AuthorityOf(c.context, tx.GetArg(AuthorityArg))
	if err != nil {
		reportErr(tx, xerrors.Errorf("incoming roster: %v", err))

		return xerrors.New(messageArgMissing)
	}

	currData, err := snap.Get(c.rosterKey)
	if err != nil {
		reportErr(tx, xerrors.Errorf("reading store: %v", err))

		return xerrors.New(messageStorageEmpty)
	}

	curr, err := c.rosterFac.AuthorityOf(c.context, currData)
	if err != nil {
		reportErr(tx, xerrors.Errorf("stored roster: %v", err))

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

	// TODO: check the number of changes per batch instead of transaction wide.

	creds := NewCreds(c.accessKey)

	err = c.access.Match(snap, creds, tx.GetIdentity())
	if err != nil {
		reportErr(tx, xerrors.Errorf("access control: %v", err))

		return xerrors.Errorf("%s: %v", messageUnauthorized, tx.GetIdentity())
	}

	err = snap.Set(c.rosterKey, tx.GetArg(AuthorityArg))
	if err != nil {
		reportErr(tx, xerrors.Errorf("writing store: %v", err))

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
