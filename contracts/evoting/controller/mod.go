package controller

import (
	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
)

// NewController returns a new controller initializer
func NewController() node.Initializer {
	return controller{}
}

// controller is an initializer with a set of commands.
//
// - implements node.Initializer
type controller struct{
}

// Build implements node.Initializer.
func (m controller) SetCommands(builder node.Builder) {

	cmd := builder.SetCommand("e-voting")
	cmd.SetDescription("... ")

	// memcoin --config /tmp/node1 e-voting initHttpServer --portNumber 8080
	sub := cmd.SetSubCommand("initHttpServer")
	sub.SetDescription("Initialize the HTTP server")
	sub.SetFlags(cli.StringFlag{
		Name:     "portNumber",
		Usage:    "port number of the HTTP server",
		Required: true,
	})

	sub.SetAction(builder.MakeAction(&initHttpServerAction{
		ElectionIdNonce: 0 ,
		ElectionIds: make([]string, 0),
	}))

	sub = cmd.SetSubCommand("createElectionTest")
	sub.SetDescription("createElectionTest")
	sub.SetAction(builder.MakeAction(&createElectionTestAction{}))

	sub = cmd.SetSubCommand("castVoteTest")
	sub.SetDescription("castVoteTest")
	sub.SetAction(builder.MakeAction(&castVoteTestAction{}))

	sub = cmd.SetSubCommand("scenarioTest")
	sub.SetDescription("scenarioTest")
	sub.SetFlags(cli.StringSliceFlag{
		Name:     "member",
		Usage:    "nodes participating in SHUFFLE",
		Required: true,
	})
	sub.SetAction(builder.MakeAction(&scenarioTestAction{}))
}

// OnStart implements node.Initializer. It creates and registers a pedersen DKG.
func (m controller) OnStart(ctx cli.Flags, inj node.Injector) error {
	return nil
}

// OnStop implements node.Initializer.
func (controller) OnStop(node.Injector) error {
	return nil
}