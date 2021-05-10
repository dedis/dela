package controller

import (
	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/txn/pool"
	txnPoolController "go.dedis.ch/dela/core/txn/pool/controller"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/shuffle/neff"
	"golang.org/x/xerrors"
)

// NewController returns a new controller initializer
func NewController() node.Initializer {
	return controller{}
}

// controller is an initializer with a set of commands.
//
// - implements node.Initializer
type controller struct{}


// Build implements node.Initializer.
func (m controller) SetCommands(builder node.Builder) {

	cmd := builder.SetCommand("shuffle")
	cmd.SetDescription("... ")

	sub := cmd.SetSubCommand("init")
	sub.SetDescription("Initialize the SHUFFLE protocol")
	sub.SetAction(builder.MakeAction(&initAction{}))

	// memcoin --config /tmp/node1 shuffle testNeff --member $(memcoin --config /tmp/node1 shuffle export) --member $(memcoin --config /tmp/node2 shuffle export) --member $(memcoin --config /tmp/node3 shuffle export)
	// Todo : implement tests !
	sub = cmd.SetSubCommand("testNeff")
	sub.SetDescription("Runs the neff shuffle protocol on the hardcoded ElGamal pairs")
	sub.SetFlags(cli.StringSliceFlag{
		Name:     "member",
		Usage:    "nodes participating in the SHUFFLE protocol",
		Required: true,
	})
	sub.SetAction(builder.MakeAction(&shuffleAction{}))

	sub = cmd.SetSubCommand("export")
	sub.SetDescription("Export the node Address")
	sub.SetAction(builder.MakeAction(&exportInfoAction{}))
}

// OnStart implements node.Initializer. It creates and registers a pedersen DKG.
func (m controller) OnStart(ctx cli.Flags, inj node.Injector) error {
	var no mino.Mino
	err := inj.Resolve(&no)
	if err != nil {
		return xerrors.Errorf("failed to resolve mino.Mino: %v", err)
	}

	var p pool.Pool
	err = inj.Resolve(&p)
	if err != nil {
		return xerrors.Errorf("failed to resolve pool.Pool: %v", err)
	}

	var service ordering.Service
	err = inj.Resolve(&service)
	if err != nil {
		return xerrors.Errorf("failed to resolve ordering.Service: %v", err)
	}

	var client *txnPoolController.Client
	err = inj.Resolve(&client)
	if err != nil {
		return xerrors.Errorf("failed to resolve txn pool client: %v", err)
	}

	neffShuffle := neff.NewNeffShuffle(no, service, p, client)

	inj.Inject(neffShuffle)

	return nil
}

// OnStop implements node.Initializer.
func (controller) OnStop(node.Injector) error {
	return nil
}
