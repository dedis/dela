// Package node the Builder type, which builds an CLI application to controle a
// node. The application will have a start command by default and it provide a
// function to create actions that will eventually be executed on the running
// node.
//
// 	type helloTemplate struct{}
//
// 	func (tmpl helloTemplate) GenerateRequest(flags cli.Flags) ([]byte, error) {
// 		return []byte(flags.String("dude")), nil
// 	}
//
// 	func (tmpl helloTemplate) Execute(ctx Context) error {
// 		buffer, err := ioutil.ReadAll(ctx.In)
// 		// if err != nil ...
//
// 		var hello Hello
// 		err = ctx.Injector.Resolve(&hello)
// 		// if err != nil ...
//
// 		hello.SayTo(string(buffer))
// 		return nil
// 	}
//
// The template provided to the builder will return an action that can be used
// to create a command.
//
// 	var builder Builder
// 	cmd := builder.SetCommand("hello")
// 	cmd.SetAction(builder.MakeAction(helloTemplate{}))
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

// Injector is a dependcy injection abstraction.
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

	// Inject starts the components of the initializer and populates the
	// injector.
	Inject(cli.Flags, Injector) error
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
