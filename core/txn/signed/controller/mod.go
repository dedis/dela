// Package controller implements a CLI controller to inject a transaction
// manager.
//
// Documentation Last Review: 08.10.2020
//
package controller

import (
	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/txn/signed"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/cosi"
)

// MgrController is a CLI controller that will inject a transaction manager
// using the signer of the collective signing component.
//
// - implements node.Initializer
type mgrController struct{}

// NewManagerController creates a new controller that will inject a transaction
// manager in the context.
func NewManagerController() node.Initializer {
	return mgrController{}
}

// SetCommands implements node.Initializer. It does not register any command.
func (mgrController) SetCommands(node.Builder) {}

// OnStart implements node.Initializer. It creates a transaction manager using
// the signer of the collective signing component and injects it.
func (mgrController) OnStart(flags cli.Flags, inj node.Injector) error {
	var srvc ordering.Service
	err := inj.Resolve(&srvc)
	if err != nil {
		return err
	}

	var nonceMgr validation.Service
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

// OnStop implements node.initializer. It does nothing.
func (mgrController) OnStop(node.Injector) error {
	return nil
}

// Client is a local client for the manager to read the current identity's nonce
// from the ordering service.
//
// - implements signed.Client
type client struct {
	srvc ordering.Service
	mgr  validation.Service
}

// GetNonce implements signed.Client. It reads the store of the ordering service
// to get the next nonce of the identity and returns it.
func (c client) GetNonce(ident access.Identity) (uint64, error) {
	store := c.srvc.GetStore()

	nonce, err := c.mgr.GetNonce(store, ident)
	if err != nil {
		return 0, err
	}

	return nonce, nil
}
