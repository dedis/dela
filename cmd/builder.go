package cmd

import (
	"encoding/binary"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v2"
	"go.dedis.ch/dela"
	"golang.org/x/xerrors"
)

type cliBuilder struct {
	daemonFactory DaemonFactory
	injector      Injector
	actions       *actionMap
	sigs          chan os.Signal
	controllers   []Controller
	startFlags    []Flag
	commands      []*cliCommand
}

func (b *cliBuilder) Command(name string) CommandBuilder {
	cb := &cliCommand{name: name}
	b.commands = append(b.commands, cb)
	return cb
}

func (b *cliBuilder) Start(flags ...Flag) Builder {
	b.startFlags = append(b.startFlags, flags...)
	return b
}

func (b *cliBuilder) build() []*cli.Command {
	for _, controller := range b.controllers {
		controller.Build(b)
	}

	commands := b.buildCommands(b.commands)

	commands = append(commands, &cli.Command{
		Name:   "start",
		Usage:  "start the daemon",
		Flags:  b.buildFlags(b.startFlags),
		Action: b.start,
	})

	return commands
}

func (b *cliBuilder) start(c *cli.Context) error {
	daemon, err := b.daemonFactory.DaemonFromContext(c)
	if err != nil {
		return xerrors.Errorf("couldn't make daemon: %v", err)
	}

	err = daemon.Listen()
	if err != nil {
		return xerrors.Errorf("couldn't start the daemon: %v", err)
	}

	defer daemon.Close()

	for _, controller := range b.controllers {
		err = controller.Run(c, b.injector)
		if err != nil {
			return xerrors.Errorf("couldn't run the controller: %v", err)
		}
	}

	signal.Notify(b.sigs, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(b.sigs)

	<-b.sigs

	dela.Logger.Trace().Msg("daemon has been stopped")

	return nil
}

func (b *cliBuilder) buildCommands(in []*cliCommand) []*cli.Command {
	if len(in) == 0 {
		return nil
	}

	commands := make([]*cli.Command, len(in))
	for i, command := range in {
		cmd := &cli.Command{
			Name:  command.name,
			Usage: command.description,
			Flags: b.buildFlags(command.flags),
		}

		if command.action != nil {
			b.fillAction(cmd, command.action)
		}

		cmd.Subcommands = b.buildCommands(command.subcommands)

		commands[i] = cmd
	}

	return commands
}

func (b *cliBuilder) fillAction(cmd *cli.Command, a Action) {
	index := b.actions.Set(a)

	cmd.Action = func(c *cli.Context) error {
		client, err := b.daemonFactory.ClientFromContext(c)
		if err != nil {
			return xerrors.Errorf("couldn't make client: %v", err)
		}

		id := make([]byte, 2)
		binary.LittleEndian.PutUint16(id, index)

		action := b.actions.Get(index)
		msg, err := action.Prepare(c)
		if err != nil {
			return xerrors.Errorf("couldn't prepare action: %v", err)
		}

		err = client.Send(append(id, msg...))
		if err != nil {
			return xerrors.Errorf("couldn't send action: %v", err)
		}

		return nil
	}
}

func (b *cliBuilder) buildFlags(in []Flag) []cli.Flag {
	flags := make([]cli.Flag, len(in))
	for i, input := range in {
		switch flag := input.(type) {
		case StringFlag:
			flags[i] = &cli.StringFlag{
				Name:     flag.Name,
				Usage:    flag.Usage,
				Required: flag.Required,
				Value:    flag.Value,
			}
		case DurationFlag:
			flags[i] = &cli.DurationFlag{
				Name:     flag.Name,
				Usage:    flag.Usage,
				Required: flag.Required,
				Value:    flag.Value,
			}
		case IntFlag:
			flags[i] = &cli.IntFlag{
				Name:     flag.Name,
				Usage:    flag.Usage,
				Required: flag.Required,
				Value:    flag.Value,
			}
		default:
			panic(fmt.Sprintf("flag type '%T' not supported", input))
		}
	}

	return flags
}

type cliCommand struct {
	name        string
	description string
	action      Action
	flags       []Flag
	subcommands []*cliCommand
}

func (c *cliCommand) Description(value string) CommandBuilder {
	c.description = value
	return c
}

func (c *cliCommand) Action(a Action) CommandBuilder {
	c.action = a
	return c
}

func (c *cliCommand) Flags(flags ...Flag) CommandBuilder {
	c.flags = flags
	return c
}

func (c *cliCommand) Command(name string) CommandBuilder {
	sub := &cliCommand{name: name}
	c.subcommands = append(c.subcommands, sub)
	return sub
}
