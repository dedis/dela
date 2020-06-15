package node

import (
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
	ucli "github.com/urfave/cli/v2"
	"go.dedis.ch/dela/cli"
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

	builder.daemonFactory = fakeFactory{err: xerrors.New("oops")}
	err = builder.start(nil)
	require.EqualError(t, err, "couldn't make daemon: oops")

	builder.daemonFactory = fakeFactory{errDaemon: xerrors.New("oops")}
	err = builder.start(nil)
	require.EqualError(t, err, "couldn't start the daemon: oops")

	builder = NewBuilder(fakeInitializer{err: xerrors.New("oops")}).(*cliBuilder)
	err = builder.start(nil)
	require.EqualError(t, err, "couldn't run the controller: oops")
}

func TestCliBuilder_MakeNodeAction(t *testing.T) {
	builder := &cliBuilder{
		actions:       &actionMap{},
		daemonFactory: fakeFactory{},
	}

	err := builder.MakeAction(fakeAction{})(fakeContext{})
	require.NoError(t, err)

	err = builder.MakeAction(fakeAction{err: xerrors.New("oops")})(fakeContext{})
	require.EqualError(t, err, "couldn't prepare action: oops")

	builder.daemonFactory = fakeFactory{err: xerrors.New("oops")}
	err = builder.MakeAction(fakeAction{})(fakeContext{})
	require.EqualError(t, err, "couldn't make client: oops")

	builder.daemonFactory = fakeFactory{errClient: xerrors.New("oops")}
	err = builder.MakeAction(fakeAction{})(fakeContext{})
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
	sub.SetFlags(cli.DurationFlag{}, cli.IntFlag{})

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
