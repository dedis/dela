// Package urfave provides a cli builder implementation based on the urfave/cli
// library.
package urfave

import (
	"fmt"

	ucli "github.com/urfave/cli/v2"
	"go.dedis.ch/dela/cli"
)

// Builder implements a cli builder based on urfave/cli
//
// - implements cli.Builder
type Builder struct {
	commands []*cmdBuilder
	name     string
	action   cli.Action
	flags    []cli.Flag
}

// NewBuilder returns a new initialized builder. Action allows one to define a
// primary action, but can be nil if we only needs to define commands. Flags
// provides the global flags available from all the commands/subcommands.
func NewBuilder(name string, action cli.Action, flags ...cli.Flag) cli.Builder {
	return &Builder{
		name:   name,
		action: action,
		flags:  flags,
	}
}

// Build implements cli.builder.
func (b Builder) Build() cli.Application {
	app := &ucli.App{
		Name:     b.name,
		Commands: buildCommand(b.commands),
		Action:   makeAction(b.action),
		Flags:    buildFlags(b.flags),
	}

	app.Setup()

	return app
}

// SetCommand implements cli.Builder.
func (b *Builder) SetCommand(name string) cli.CommandBuilder {
	cmd := &cmdBuilder{
		name: name,
	}
	b.commands = append(b.commands, cmd)

	return cmd
}

// commandBuilder is the struct provided to build commands.
//
// - implements cli.CommandBuilder
type cmdBuilder struct {
	name        string
	description string
	action      cli.Action
	flags       []ucli.Flag
	subcommands []*cmdBuilder
}

// SetDescription implements cli.CommandBuilder.
func (b *cmdBuilder) SetDescription(value string) {
	b.description = value
}

// SetFlags implements cli.CommandBuilder.
func (b *cmdBuilder) SetFlags(flags ...cli.Flag) {
	b.flags = buildFlags(flags)
}

// SetAction implements cli.CommandBuilder.
func (b *cmdBuilder) SetAction(action cli.Action) {
	b.action = action
}

// SetSubCommand implements cli.CommandBuilder.
func (b *cmdBuilder) SetSubCommand(name string) cli.CommandBuilder {
	builder := &cmdBuilder{
		name: name,
	}
	b.subcommands = append(b.subcommands, builder)

	return builder
}

// buildFlags converts cli.Flag to their corresponding urfave/cli.
func buildFlags(flags []cli.Flag) []ucli.Flag {
	res := make([]ucli.Flag, len(flags))

	for i, f := range flags {
		var flag ucli.Flag

		switch e := f.(type) {
		case cli.StringFlag:
			flag = &ucli.StringFlag{
				Name:     e.Name,
				Usage:    e.Usage,
				Required: e.Required,
				Value:    e.Value,
			}
		case cli.StringSliceFlag:
			flag = &ucli.StringSliceFlag{
				Name:     e.Name,
				Usage:    e.Usage,
				Required: e.Required,
				Value:    ucli.NewStringSlice(e.Value...),
			}
		case cli.DurationFlag:
			flag = &ucli.DurationFlag{
				Name:     e.Name,
				Usage:    e.Usage,
				Required: e.Required,
				Value:    e.Value,
			}
		case cli.IntFlag:
			flag = &ucli.IntFlag{
				Name:     e.Name,
				Usage:    e.Usage,
				Required: e.Required,
				Value:    e.Value,
			}
		default:
			panic(fmt.Sprintf("flag type '%T' not supported", f))
		}

		res[i] = flag
	}

	return res
}

// buildCommand recursively builds the commands from a cmdBuilder struct to a
// ucli commands.
func buildCommand(cmds []*cmdBuilder) []*ucli.Command {
	commands := make([]*ucli.Command, len(cmds))

	for i, cmd := range cmds {
		commands[i] = &ucli.Command{
			Name:        cmd.name,
			Usage:       cmd.description,
			Action:      makeAction(cmd.action),
			Flags:       cmd.flags,
			Subcommands: buildCommand(cmd.subcommands),
		}
	}

	return commands
}

// makeAction transforms a cli.Action to its urfave form.
func makeAction(action cli.Action) ucli.ActionFunc {
	if action != nil {
		return func(ctx *ucli.Context) error {
			return action(ctx)
		}
	}
	return nil
}
