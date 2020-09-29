package controller

import (
	"os"
	"os/signal"

	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/mino/proxy/http"
)

const defaultAddr = "127.0.0.1:8080"

// NewMinimal returns a new minimal initializer
func NewMinimal() node.Initializer {
	return minimal{}
}

// minimal is an initializer with the minimum set of commands. Indeed it only
// creates and injects a new client proxy
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

// Inject implements node.Initializer. It creates, starts, and registers a
// client proxy.
func (m minimal) Inject(ctx cli.Flags, inj node.Injector) error {

	addr := ctx.String("clientaddr")

	proxyhttp := http.NewHTTP(addr)

	inj.Inject(proxyhttp)

	go proxyhttp.Listen()

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, os.Interrupt)
		<-quit
		proxyhttp.Stop()
	}()

	return nil
}
