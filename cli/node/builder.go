// This file contains the implementation of a CLI builder.
//
// Documentation Last Review: 13.10.20202
//

package node

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"os/signal"
	"syscall"

	urfave "github.com/urfave/cli/v2"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/ucli"
	"golang.org/x/xerrors"
)

// CLIBuilder is an application builder that will build a CLI to start and
// control a node.
//
// - implements node.Builder
// - implements cli.Builder
type CLIBuilder struct {
	cli.Builder

	daemonFactory DaemonFactory
	injector      Injector
	actions       *actionMap
	startFlags    []cli.Flag
	inits         []Initializer
	writer        io.Writer

	// In production, the daemon is stopped via SIGTERM. In case of testing, the
	// channel will be closed instead, because of instability.
	enableSignal bool
	sigs         chan os.Signal
}

// NewBuilder returns a new empty builder.
func NewBuilder(inits ...Initializer) *CLIBuilder {
	return NewBuilderWithCfg(nil, nil, inits...)
}

// NewBuilderWithCfg returns a new empty builder with specific configurations.
func NewBuilderWithCfg(sigs chan os.Signal, out io.Writer, inits ...Initializer) *CLIBuilder {
	if out == nil {
		out = os.Stdout
	}

	enabled := false

	if sigs == nil {
		sigs = make(chan os.Signal, 1)
		enabled = true
	}

	injector := NewInjector()

	actions := &actionMap{}

	factory := socketFactory{
		injector: injector,
		actions:  actions,
		out:      out,
	}

	// We are using urfave cli builder
	builder := ucli.NewBuilder("Dela", nil, cli.StringFlag{
		Name:  "config",
		Usage: "path to the config folder",
		Value: ".dela",
	})

	return &CLIBuilder{
		Builder:       builder,
		injector:      injector,
		actions:       actions,
		daemonFactory: factory,
		enableSignal:  enabled,
		sigs:          sigs,
		inits:         inits,
		writer:        out,
	}
}

// SetStartFlags implements node.Builder. It appends the given flags to the list
// of flags that will be used to create the start command.
func (b *CLIBuilder) SetStartFlags(flags ...cli.Flag) {
	b.startFlags = append(b.startFlags, flags...)
}

// MakeAction implements node.Builder. It creates a CLI action from the
// template.
func (b *CLIBuilder) MakeAction(tmpl ActionTemplate) cli.Action {
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
		lookupFlags(fset, c.(*urfave.Context))

		buf, err := json.Marshal(fset)
		if err != nil {
			return xerrors.Errorf("failed to marshal flag set: %v", err)
		}

		err = client.Send(append(id, buf...))
		if err != nil {
			return xerrors.Opaque(err)
		}

		return nil
	}
}

func lookupFlags(fset FlagSet, ctx *urfave.Context) {
	for _, ancestor := range ctx.Lineage() {
		if ancestor.Command != nil {
			fill(fset, ancestor.Command.Flags, ancestor)
		}

		if ancestor.App != nil {
			fill(fset, ancestor.App.Flags, ancestor)
		}
	}
}

func fill(fset FlagSet, flags []urfave.Flag, ctx *urfave.Context) {
	for _, flag := range flags {
		names := flag.Names()
		if len(names) > 0 {
			fset[names[0]] = convert(ctx.Value(names[0]))
		}
	}
}

func convert(v interface{}) interface{} {
	switch value := v.(type) {
	case urfave.StringSlice:
		// StringSlice is an edge-case as it won't serialize correctly with JSON
		// so we ask for the actual []string to allow a correct serialization.
		return value.Value()
	default:
		return v
	}
}

// Build implements node.Builder. It returns the application.
func (b *CLIBuilder) Build() cli.Application {
	for _, controller := range b.inits {
		controller.SetCommands(b)
	}

	cmd := b.SetCommand("start")
	cmd.SetDescription("start the deamon")
	cmd.SetFlags(b.startFlags...)
	cmd.SetAction(b.start)

	return b.Builder.Build()
}

func (b *CLIBuilder) start(flags cli.Flags) error {
	if b.enableSignal {
		signal.Notify(b.sigs, syscall.SIGINT, syscall.SIGTERM)

		defer signal.Stop(b.sigs)
	}

	dir := flags.Path("config")
	if dir != "" {
		err := os.MkdirAll(dir, 0700)
		if err != nil {
			return xerrors.Errorf("couldn't make path: %v", err)
		}
	}

	daemon, err := b.daemonFactory.DaemonFromContext(flags)
	if err != nil {
		return xerrors.Errorf("couldn't make daemon: %v", err)
	}

	for _, controller := range b.inits {
		err = controller.OnStart(flags, b.injector)
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

	<-b.sigs
	signal.Stop(b.sigs)

	// Controllers are stopped in reverse order so that high level components
	// are stopped before lower level ones (i.e. stop a service before the
	// database to avoid errors).
	for i := len(b.inits) - 1; i >= 0; i-- {
		err = b.inits[i].OnStop(b.injector)
		if err != nil {
			return xerrors.Errorf("couldn't stop controller: %v", err)
		}
	}

	dela.Logger.Trace().Msg("daemon has been stopped")

	return nil
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
