package node

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
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
	writer        io.Writer
}

// NewBuilder returns a new empty builder.
func NewBuilder(inits ...Initializer) cli.Builder {
	return NewBuilderWithCfg(nil, nil, inits...)
}

// NewBuilderWithCfg returns a new empty builder with specific configurations.
func NewBuilderWithCfg(sigs chan os.Signal, out io.Writer, inits ...Initializer) cli.Builder {
	if out == nil {
		out = os.Stdout
	}

	if sigs == nil {
		sigs = make(chan os.Signal, 1)
	}

	injector := &reflectInjector{
		mapper: make(map[reflect.Type]interface{}),
	}

	actions := &actionMap{}

	factory := socketFactory{
		injector: injector,
		actions:  actions,
		out:      out,
	}

	return &cliBuilder{
		injector:      injector,
		actions:       actions,
		daemonFactory: factory,
		sigs:          sigs,
		inits:         inits,
		writer:        out,
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

		// Prepare a set of flags that will be transmitted to the daemon so that
		// the action has access to the same flags and their values.
		fset := make(FlagSet)
		lookupFlags(fset, c.(*ucli.Context))

		buf, err := json.Marshal(fset)
		if err != nil {
			return xerrors.Errorf("failed to marshal flag set: %v", err)
		}

		err = client.Send(append(id, buf...))
		if err != nil {
			return xerrors.Errorf("couldn't send action: %v", err)
		}

		return nil
	}
}

func lookupFlags(fset FlagSet, ctx *ucli.Context) {
	for _, ancestor := range ctx.Lineage() {
		if ancestor.Command != nil {
			fill(fset, ancestor.Command.Flags, ancestor)
		}

		if ancestor.App != nil {
			fill(fset, ancestor.App.Flags, ancestor)
		}
	}
}

func fill(fset FlagSet, flags []ucli.Flag, ctx *ucli.Context) {
	for _, flag := range flags {
		names := flag.Names()
		if len(names) > 0 {
			fset[names[0]] = convert(ctx.Value(names[0]))
		}
	}
}

func convert(v interface{}) interface{} {
	switch value := v.(type) {
	case ucli.StringSlice:
		// StringSlice is an edge-case as it won't serialize correctly with JSON
		// so we ask for the actual []string to allow a correct serialization.
		return value.Value()
	default:
		return v
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
		Writer:   b.writer,
	}

	return app
}

func (b *cliBuilder) start(c *ucli.Context) error {
	dir := c.Path("config")
	if dir != "" {
		err := os.MkdirAll(dir, 0700)
		if err != nil {
			return xerrors.Errorf("couldn't make path: %v", err)
		}
	}

	daemon, err := b.daemonFactory.DaemonFromContext(c)
	if err != nil {
		return xerrors.Errorf("couldn't make daemon: %v", err)
	}

	for _, controller := range b.inits {
		err = controller.OnStart(c, b.injector)
		if err != nil {
			return xerrors.Errorf("couldn't run the controller: %v", err)
		}
	}

	// Daemon is started after the controllers so that everything has started
	// when the daemon is available.
	err = daemon.Listen()
	if err != nil {
		return xerrors.Errorf("couldn't start the daemon: %v", err)
	}

	defer daemon.Close()

	signal.Notify(b.sigs, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(b.sigs)

	<-b.sigs

	for i := len(b.inits) - 1; i >= 0; i-- {
		err = b.inits[i].OnStop(b.injector)
		if err != nil {
			return xerrors.Errorf("couldn't stop controller: %v", err)
		}
	}

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
