package cmd

import (
	"io"
	"time"
)

// Application is the main interface to run the CLI.
type Application interface {
	Run(arguments []string) error
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
	ClientFromContext(Context) (Client, error)
	DaemonFromContext(Context) (Daemon, error)
}

// Injector is a dependcy injection abstraction.
type Injector interface {
	// Resolve populates the input with the dependency if any compatible exists.
	Resolve(interface{}) error

	// Inject stores the dependency to be resolved later on.
	Inject(interface{})
}

// Controller is the interface to implement to enable commands specific to
// the ledger implementation.
type Controller interface {
	// Build populates the builder with the commands of the controller.
	Build(Builder)

	// Run starts the components of the controller and populates the injector.
	Run(Context, Injector) error
}

// Request is a context to handle commands sent to the daemon.
type Request struct {
	Injector Injector
	In       io.Reader
	Out      io.Writer
}

// Context is provide to an action to read the flags.
type Context interface {
	String(name string) string
	Duration(name string) time.Duration
	Path(name string) string
	Int(name string) int
}

// Action is the definition of a command.
type Action interface {
	// Prepare returns the bytes to be sent to the daemon.
	Prepare(Context) ([]byte, error)

	// Execute processes a command received from the CLI on the daemon.
	Execute(Request) error
}

// Builder defines how to build a list of commands to tweak the ledger.
type Builder interface {
	// Command populates a new command.
	Command(name string) CommandBuilder

	Start(...Flag) Builder
}

// Flag is a identifier for the definition of the flags.
type Flag interface {
	Flag()
}

// CommandBuilder defines how to build a command.
type CommandBuilder interface {
	// Description sets the value of the description for this command.
	Description(value string) CommandBuilder

	// Action sets the action for this command.
	Action(Action) CommandBuilder

	// Flags sets the flags for this command.
	Flags(...Flag) CommandBuilder

	// Command populates a subcommand for this command.
	Command(name string) CommandBuilder
}
