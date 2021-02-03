// Package controller implements a controller for the access contract.
//
// Documentation Last Review: 02.02.2021
//
package controller

import (
	"path/filepath"

	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
	accessContract "go.dedis.ch/dela/contracts/access"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/execution/native"
	"golang.org/x/xerrors"
)

var aKey = [32]byte{1}

// newStore is the function used to create the new store. It allows us to create
// a different store in the tests.
var newStore = func(path string) (accessStore, error) {
	return newJstore(path)
}

// miniController is a CLI initializer to allow a user to grant access to the
// access contract.
//
// - implements node.Initializer
type miniController struct{}

// NewController creates a new minimal controller for the access contract.
func NewController() node.Initializer {
	return miniController{}
}

// SetCommands implements node.Initializer. It sets the command to control the
// service.
func (miniController) SetCommands(builder node.Builder) {
	cmd := builder.SetCommand("access")
	cmd.SetDescription("Handles the access contract")

	sub := cmd.SetSubCommand("add")
	sub.SetDescription("add an identity")
	sub.SetFlags(cli.StringSliceFlag{
		Name:     "identity",
		Usage:    "identity to add, in the form of bls public keys",
		Required: true,
	})
	sub.SetAction(builder.MakeAction(addAction{}))
}

// OnStart implements node.Initializer. It registers the access contract.
func (m miniController) OnStart(flags cli.Flags, inj node.Injector) error {
	var access access.Service
	err := inj.Resolve(&access)
	if err != nil {
		return xerrors.Errorf("failed to resolve access service: %v", err)
	}

	var exec *native.Service
	err = inj.Resolve(&exec)
	if err != nil {
		return xerrors.Errorf("failed to resolve native service: %v", err)
	}

	accessStore, err := newStore(filepath.Join(flags.String("config"), "access.json"))
	if err != nil {
		return xerrors.Errorf("failed to create access store: %v", err)
	}

	contract := accessContract.NewContract(aKey[:], access, accessStore)
	accessContract.RegisterContract(exec, contract)

	inj.Inject(accessStore)

	return nil
}

// OnStop implements node.Initializer.
func (miniController) OnStop(inj node.Injector) error {
	return nil
}
