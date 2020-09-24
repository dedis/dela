package node

import (
	"flag"
	"runtime"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
	ucli "github.com/urfave/cli/v2"
	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/internal/testing/fake"
	"golang.org/x/xerrors"
)

func TestCliBuilder_Command(t *testing.T) {
	builder := &cliBuilder{}

	cmd := builder.SetCommand("test")
	require.NotNil(t, cmd)
	require.Len(t, builder.commands, 1)
}

func TestCliBuilder_SetStartFlags(t *testing.T) {
	builder := &cliBuilder{}

	builder.SetStartFlags(cli.StringFlag{}, cli.IntFlag{})
	require.Len(t, builder.startFlags, 2)
}

func TestCliBuilder_Start(t *testing.T) {
	builder := NewBuilder(fakeInitializer{}).(*cliBuilder)
	builder.sigs <- syscall.SIGTERM

	err := builder.start(nil)
	require.NoError(t, err)

	fset := flag.NewFlagSet("", 0)
	fset.String("config", "/test/", "")

	if runtime.GOOS != "windows" {
		ctx := ucli.NewContext(nil, fset, nil)
		err = builder.start(ctx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "couldn't make path: mkdir /test/: ")
	}

	builder.daemonFactory = fakeFactory{err: xerrors.New("oops")}
	err = builder.start(nil)
	require.EqualError(t, err, "couldn't make daemon: oops")

	builder.daemonFactory = fakeFactory{errDaemon: xerrors.New("oops")}
	err = builder.start(nil)
	require.EqualError(t, err, "couldn't start the daemon: oops")

	// Test when a component cannot start.
	builder = NewBuilder(fakeInitializer{err: xerrors.New("oops")}).(*cliBuilder)
	builder.sigs <- syscall.SIGTERM

	err = builder.start(nil)
	require.EqualError(t, err, "couldn't run the controller: oops")

	// Test when a component cannot stop.
	builder = NewBuilder(fakeInitializer{errStop: xerrors.New("oops")}).(*cliBuilder)
	builder.sigs <- syscall.SIGTERM

	err = builder.start(nil)
	require.EqualError(t, err, "couldn't stop controller: oops")
}

func TestCliBuilder_MakeAction(t *testing.T) {
	calls := &fake.Call{}
	builder := &cliBuilder{
		actions:       &actionMap{},
		daemonFactory: fakeFactory{calls: calls},
	}

	fset := flag.NewFlagSet("", 0)
	fset.Var(ucli.NewStringSlice("item 1", "item 2"), "flag-1", "")
	fset.Int("flag-2", 20, "")

	ctx := ucli.NewContext(makeApp(), fset, nil)

	err := builder.MakeAction(fakeAction{})(ctx)
	require.NoError(t, err)

	data := string(calls.Get(0, 0).([]byte))
	require.Equal(t, "\x00\x00"+`{"flag-1":["item 1","item 2"],"flag-2":20}`, data)

	builder.daemonFactory = fakeFactory{err: xerrors.New("oops")}
	err = builder.MakeAction(fakeAction{})(ctx)
	require.EqualError(t, err, "couldn't make client: oops")

	builder.daemonFactory = fakeFactory{errClient: xerrors.New("oops")}
	err = builder.MakeAction(fakeAction{})(ctx)
	require.EqualError(t, err, "couldn't send action: oops")
}

func TestCliBuilder_Build(t *testing.T) {
	builder := &cliBuilder{
		actions:       &actionMap{},
		daemonFactory: fakeFactory{},
		inits:         []Initializer{fakeInitializer{}},
	}

	cb := builder.SetCommand("test")
	cb.SetDescription("test description")
	cb.SetAction(builder.MakeAction(fakeAction{}))
	cb.SetFlags(cli.StringFlag{Name: "string-flag"})

	sub := cb.SetSubCommand("subtest")
	sub.SetDescription("subtest description")
	sub.SetFlags(cli.DurationFlag{}, cli.IntFlag{}, cli.StringSliceFlag{})

	cb = builder.SetCommand("another")
	cb.SetAction(func(cli.Flags) error {
		return nil
	})

	cb = builder.SetCommand("last")
	cb.SetAction(func(cli.Flags) error {
		return xerrors.New("oops")
	})

	app := builder.Build().(*ucli.App)
	require.Len(t, app.Commands, 4)

	// Check the referencing of the actions.
	err := app.Commands[1].Action(nil)
	require.NoError(t, err)
}

func TestCliBuilder_UnknownType_BuildFlags(t *testing.T) {
	defer func() {
		r := recover()
		require.Equal(t, "flag type '<nil>' not supported", r)
	}()

	builder := &cliBuilder{}
	builder.SetStartFlags((cli.Flag)(nil))

	builder.buildFlags(builder.startFlags)
}

// -----------------------------------------------------------------------------
// Utility functions

func makeApp() *ucli.App {
	return &ucli.App{
		Flags: []ucli.Flag{
			&ucli.StringSliceFlag{Name: "flag-1"},
			&ucli.IntFlag{Name: "flag-2"},
		},
	}
}
