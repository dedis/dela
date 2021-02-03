// Package cli defines the Builder type, which allows one to build a CLI
// application in a modular way.
//
// 	var builder Builder
// 	builder.SetName("myapp")
//
// 	cmd := builder.SetCommand("hello")
// 	cmd.SetDescription("Say hello !")
// 	cmd.SetAction(func(flags Flags) error {
// 		fmt.Printf("Hello %s!\n", flags.String("dude"))
// 	})
//
// 	builder.Build().Run(os.Args)
//
// An implementation of the builder is free to provide primitives to create more
// complex action.
//
// Documentation Last Review: 13.10.2020
//
package cli

import (
	"time"
)

// Builder is an application builder interface. One can set properties of an
// application then build it.
type Builder interface {
	Provider

	// Build returns the application.
	Build() Application
}

// Application is the main interface to run the CLI.
type Application interface {
	Run(arguments []string) error
}

// CommandBuilder is a command builder interface. One can set properties of a
// specific command like its name and description and what it should do when
// invoked.
type CommandBuilder interface {
	// SetDescription sets the value of the description for this command.
	SetDescription(value string)

	// SetFlags sets the flags for this command.
	SetFlags(...Flag)

	// SetAction sets the action for this command.
	SetAction(Action)

	// SetSubCommand creates a subcommand for this command.
	SetSubCommand(name string) CommandBuilder
}

// Action is a function that will be executed when a command is invoked.
type Action func(Flags) error

// Flag is an identifier for the definition of the flags.
type Flag interface {
	Flag()
}

// Flags provides the primitives to an action to read the flags.
type Flags interface {
	String(name string) string

	StringSlice(name string) []string

	Duration(name string) time.Duration

	Path(name string) string

	Int(name string) int

	Bool(name string) bool
}

// Initializer defines a primitive for modules to add their commands. A cli will
// gather all the initializers from each desired modules and call the
// SetCommands for each of them.
type Initializer interface {
	// SetCommands if the function called by the builder to add the modules'
	// commands. The modules implement this function and use the provided
	// provider to create its specific commands.
	SetCommands(Provider)
}

// Provider defines a primitive for modules to provide their commands
type Provider interface {
	// SetCommand creates a new command with the given name and returns its
	// builder.
	SetCommand(name string) CommandBuilder
}
