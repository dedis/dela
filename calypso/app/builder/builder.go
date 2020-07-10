package builder

import (
	ucli "github.com/urfave/cli/v2"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/cli"
)

// Builder builds the Application
//
// - implements cli.Builder
type Builder struct {
	app      *ucli.App
	commands []*CommandBuilder
}

// SetCommand implements cli.Builder
func (b *Builder) SetCommand(name string) cli.CommandBuilder {
	cb := &CommandBuilder{Name: name}
	b.commands = append(b.commands, cb)
	return cb
}

// NewBuilder creates a new Builder
func NewBuilder() cli.Builder {
	return &Builder{}
}

// Build implements cli.Builder. It returns an application based on the urfave
// cli.
func (b *Builder) Build() cli.Application {

	commands := make([]*ucli.Command, len(b.commands))
	for i, cb := range b.commands {
		command := &ucli.Command{
			Name:        cb.Name,
			Description: cb.Description,
			Action:      cb.Action,
			Flags:       cb.Flags,
		}
		commands[i] = command
	}

	app := &ucli.App{
		Name:     "Calypso",
		Usage:    "Secret sharing",
		Flags:    []ucli.Flag{},
		Commands: commands,
	}

	b.app = app

	return app
}

// CommandBuilder builds the command
//
// - implements cli.CommandBuilder
type CommandBuilder struct {
	Name        string
	Action      ucli.ActionFunc
	Flags       []ucli.Flag
	Description string
}

// SetDescription implements cli.CommandBuilder
func (cb *CommandBuilder) SetDescription(description string) {
	cb.Description = description
}

// SetFlags immplements cli.CommandBuilder
func (cb *CommandBuilder) SetFlags(flags ...cli.Flag) {
	ucliflags := make([]ucli.Flag, len(flags))

	for i, flag := range flags {
		switch f := flag.(type) {
		case cli.StringFlag:
			ucliflags[i] = &ucli.StringFlag{
				Name:     f.Name,
				Usage:    f.Usage,
				Value:    f.Value,
				Required: f.Required,
			}
		default:
			dela.Logger.Fatal().Msgf("flag type '%T' not supported", flag)
		}
	}

	cb.Flags = ucliflags
}

// SetAction implements cli.CommandBuilder
func (cb *CommandBuilder) SetAction(action cli.Action) {
	cb.Action = func(c *ucli.Context) error {
		return action(c)
	}
}

// SetSubCommand implements cli.CommandBuilder
func (cb CommandBuilder) SetSubCommand(name string) cli.CommandBuilder {
	return nil
}
