// Package node defines the Builder type, which builds an CLI application to
// controle a node.
//
// The application will have a start command by default and it provides a
// function to create actions that will eventually be executed on the running
// node. See the example.
//
// Document Last Review: 13.10.2020
//
package node

import (
	"io"

	"go.dedis.ch/dela/cli"
)

// Builder is the builder that will be provided to the initializers, which can
// create commands and actions.
type Builder interface {
	// SetCommand creates a new command and returns its builder.
	SetCommand(name string) cli.CommandBuilder

	// SetStartFlags appends a list of flags that will be used to create the
	// start command.
	SetStartFlags(...cli.Flag)

	// MakeAction creates a CLI action from a given template. The template must
	// implements the handler that will be executed on the daemon.
	MakeAction(ActionTemplate) cli.Action
}

// ActionTemplate is an extension of the cli.Action interface to allow an action
// to send a request to the daemon.
type ActionTemplate interface {
	// Execute processes a command received from the CLI on the daemon.
	Execute(Context) error
}

// Context is the context available to the action when being invoked. It
// provides the dependency injector alongside with the input and output.
type Context struct {
	Injector Injector
	Flags    cli.Flags
	Out      io.Writer
}

// Injector is a dependency injection abstraction.
type Injector interface {
	// Resolve populates the input with the dependency if any compatible exists.
	Resolve(interface{}) error

	// Inject stores the dependency to be resolved later on.
	Inject(interface{})
}

// Initializer is the interface that a module can implement to set its own
// commands and inject the dependencies that will be resolved in the actions.
type Initializer interface {
	// Build populates the builder with the commands of the controller.
	SetCommands(Builder)

	// OnStart starts the components of the initializer and populates the
	// injector.
	OnStart(cli.Flags, Injector) error

	// OnStop stops the components and cleans the resources.
	OnStop(Injector) error
}

// Client is the interface to send a message to the daemon.
type Client interface {
	Send([]byte) error
}

// Daemon is an IPC socket to communicate between a CLI and a node running.
type Daemon interface {
	Listen() error
	Close() error
}

// DaemonFactory is an interface to create a daemon and clients to connect to
// it.
type DaemonFactory interface {
	ClientFromContext(cli.Flags) (Client, error)
	DaemonFromContext(cli.Flags) (Daemon, error)
}
