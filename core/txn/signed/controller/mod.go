package controller

import (
	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/txn/signed"
	"go.dedis.ch/dela/cosi"
)

// NonceManager is a required interface for the manager to read the nonce of an
// identity from the storage.
type NonceManager interface {
	GetNonce(store.Readable, access.Identity) (uint64, error)
}

type mgrController struct{}

// NewManagerController creates a new controller that will inject a transaction
// manager in the context.
func NewManagerController() node.Initializer {
	return mgrController{}
}

func (mgrController) SetCommands(node.Builder) {}

func (mgrController) Inject(flags cli.Flags, inj node.Injector) error {
	var srvc ordering.Service
	err := inj.Resolve(&srvc)
	if err != nil {
		return err
	}

	var nonceMgr NonceManager
	err = inj.Resolve(&nonceMgr)
	if err != nil {
		return err
	}

	var c cosi.CollectiveSigning
	err = inj.Resolve(&c)
	if err != nil {
		return err
	}

	mgr := signed.NewManager(c.GetSigner(), client{
		srvc: srvc,
		mgr:  nonceMgr,
	})

	inj.Inject(mgr)

	return nil
}

type client struct {
	srvc ordering.Service
	mgr  NonceManager
}

func (c client) GetNonce(ident access.Identity) (uint64, error) {
	store := c.srvc.GetStore()

	nonce, err := c.mgr.GetNonce(store, ident)
	if err != nil {
		return 0, err
	}

	return nonce, nil
}
