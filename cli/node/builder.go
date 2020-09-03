package node

import (
	"encoding/binary"
	"fmt"
	"os"
	"os/signal"
	"reflect"
	"syscall"

	ucli "github.com/urfave/cli/v2"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/cli"
	"golang.org/x/xerrors"
)

// CLIBuilder is an application builder that will build a CLI to start and
// control a node.
//
// - implements node.Builder
type cliBuilder struct {
	daemonFactory DaemonFactory
	injector      Injector
	actions       *actionMap
	sigs          chan os.Signal
	startFlags    []cli.Flag
	commands      []*cliCommand
	inits         []Initializer
}

// NewBuilder returns a new empty builder.
func NewBuilder(inits ...Initializer) cli.Builder {
	injector := &reflectInjector{
		mapper: make(map[reflect.Type]interface{}),
	}

	actions := &actionMap{}

	factory := socketFactory{
		injector: injector,
		actions:  actions,
	}

	return &cliBuilder{
		injector:      injector,
		actions:       actions,
		daemonFactory: factory,
		sigs:          make(chan os.Signal, 1),
		inits:         inits,
	}
}

// SetCommand implements node.Builder. It creates a new command and return its
// builder.
func (b *cliBuilder) SetCommand(name string) cli.CommandBuilder {
	cb := &cliCommand{name: name}
	b.commands = append(b.commands, cb)
	return cb
}

// SetStartFlags implements node.Builder. It appends the given flags to the list
// of flags that will be used to create the start command.
func (b *cliBuilder) SetStartFlags(flags ...cli.Flag) {
	b.startFlags = append(b.startFlags, flags...)
}

// MakeAction implements node.Builder. It creates a CLI action from the
// template.
func (b *cliBuilder) MakeAction(tmpl ActionTemplate) cli.Action {
	index := b.actions.Set(tmpl)

	return func(c cli.Flags) error {
		client, err := b.daemonFactory.ClientFromContext(c)
		if err != nil {
			return xerrors.Errorf("couldn't make client: %v", err)
		}

		// Encode the action ID over 2 bytes.
		id := make([]byte, 2)
		binary.LittleEndian.PutUint16(id, index)

		action := b.actions.Get(index)
		msg, err := action.GenerateRequest(c)
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

// Build implements node.Builder. It returns the application.
func (b *cliBuilder) Build() cli.Application {
	for _, controller := range b.inits {
		controller.SetCommands(b)
	}

	commands := b.buildCommands(b.commands)

	commands = append(commands, &ucli.Command{
		Name:   "start",
		Usage:  "start the daemon",
		Flags:  b.buildFlags(b.startFlags),
		Action: b.start,
	})

	app := &ucli.App{
		Name:  "Dela",
		Usage: "Dedis Ledger Architecture",
		Flags: []ucli.Flag{
			&ucli.PathFlag{
				Name:  "config",
				Usage: "path to the config folder",
				Value: ".dela",
			},
		},
		Commands: commands,
	}

	return app
}

func (b *cliBuilder) start(c *ucli.Context) error {
	daemon, err := b.daemonFactory.DaemonFromContext(c)
	if err != nil {
		return xerrors.Errorf("couldn't make daemon: %v", err)
	}

	err = daemon.Listen()
	if err != nil {
		return xerrors.Errorf("couldn't start the daemon: %v", err)
	}

	defer daemon.Close()

	for _, controller := range b.inits {
		err = controller.Inject(c, b.injector)
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

func (b *cliBuilder) buildCommands(in []*cliCommand) []*ucli.Command {
	if len(in) == 0 {
		return nil
	}

	commands := make([]*ucli.Command, len(in))
	for i, command := range in {
		cmd := &ucli.Command{
			Name:  command.name,
			Usage: command.description,
			Flags: b.buildFlags(command.flags),
		}

		if command.action != nil {
			cmd.Action = b.buildAction(command.action)
		}

		cmd.Subcommands = b.buildCommands(command.subcommands)

		commands[i] = cmd
	}

	return commands
}

func (b *cliBuilder) buildAction(a cli.Action) func(*ucli.Context) error {
	return func(ctx *ucli.Context) error {
		return a(ctx)
	}
}

func (b *cliBuilder) buildFlags(in []cli.Flag) []ucli.Flag {
	flags := make([]ucli.Flag, len(in))
	for i, input := range in {
		switch flag := input.(type) {
		case cli.StringFlag:
			flags[i] = &ucli.StringFlag{
				Name:     flag.Name,
				Usage:    flag.Usage,
				Required: flag.Required,
				Value:    flag.Value,
			}
		case cli.StringSliceFlag:
			flags[i] = &ucli.StringSliceFlag{
				Name:     flag.Name,
				Usage:    flag.Usage,
				Required: flag.Required,
				Value:    ucli.NewStringSlice(flag.Value...),
			}
		case cli.DurationFlag:
			flags[i] = &ucli.DurationFlag{
				Name:     flag.Name,
				Usage:    flag.Usage,
				Required: flag.Required,
				Value:    flag.Value,
			}
		case cli.IntFlag:
			flags[i] = &ucli.IntFlag{
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

// CLICommand is a command builder to set the properties of a command.
//
// - implements cli.CommandBuilder
type cliCommand struct {
	name        string
	description string
	action      cli.Action
	flags       []cli.Flag
	subcommands []*cliCommand
}

// SetDescription implements cli.CommandBuilder. It sets the description of the
// command.
func (c *cliCommand) SetDescription(value string) {
	c.description = value
}

// SetAction implements cli.CommandBuilder. It sets the action to invoked for
// the command.
func (c *cliCommand) SetAction(a cli.Action) {
	c.action = a
}

// SetFlags implements cli.CommandBuilder. It defines the list of flags that can
// be passed by the client.
func (c *cliCommand) SetFlags(flags ...cli.Flag) {
	c.flags = flags
}

// SetSubCommand implements cli.CommandBuilder. It creates a new subcommand and
// returns its builder.
func (c *cliCommand) SetSubCommand(name string) cli.CommandBuilder {
	sub := &cliCommand{name: name}
	c.subcommands = append(c.subcommands, sub)

	return sub
}

// ActionMap stores actions and assigns a unique index to each.
type actionMap struct {
	list []ActionTemplate
}

func (m *actionMap) Set(a ActionTemplate) uint16 {
	m.list = append(m.list, a)
	return uint16(len(m.list) - 1)
}

func (m *actionMap) Get(index uint16) ActionTemplate {
	if int(index) >= len(m.list) {
		return nil
	}

	return m.list[index]
}
