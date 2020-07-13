package controller

import (
	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/mino/httpclient"
)

const defaultAddr = "127.0.0.1:8080"

// NewMinimal returns a new minimal initializer
func NewMinimal() node.Initializer {
	return minimal{}
}

// minimal is an initializer with the minimum set of commands. Indeed it only
// creates and injects a new httpclient
//
// - implements node.Initializer
type minimal struct{}

// Build implements node.Initializer. In this case we don't need any command.
func (m minimal) SetCommands(builder node.Builder) {
	builder.SetStartFlags(cli.StringFlag{
		Name:     "clientaddr",
		Required: false,
		Usage:    "the adress of the http client",
		Value:    defaultAddr,
	})
}

// Inject implements node.Initializer. It creates, starts, and registers an
// httpclient.
func (m minimal) Inject(ctx cli.Flags, inj node.Injector) error {

	addr := ctx.String("clientaddr")

	httpclient := httpclient.NewClient(addr)

	inj.Inject(httpclient)

	go httpclient.Start()

	return nil
}
