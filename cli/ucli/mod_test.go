package ucli

import (
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	urfave "github.com/urfave/cli/v2"
	"go.dedis.ch/dela/cli"
)

func TestBuild(t *testing.T) {
	builder := NewBuilder("test", nil)
	app := builder.Build().(*urfave.App)

	app.Writer = io.Discard

	require.Equal(t, "test", app.Name)

	err := app.Run([]string{"test"})
	require.NoError(t, err)
}

func TestSetCommand(t *testing.T) {
	builder := NewBuilder("test", nil)

	builder.SetCommand("first")
	builder.SetCommand("second")

	app := builder.Build().(*urfave.App)

	require.Len(t, app.Commands, 3)

	require.Equal(t, "first", app.Commands[0].Name)
	require.Equal(t, "second", app.Commands[1].Name)
	require.Equal(t, "help", app.Commands[2].Name)

}

func TestCommandBuilder(t *testing.T) {
	builder := NewBuilder("test", nil).(*Builder)
	cmd := builder.SetCommand("first")

	fakeAction := func(flags cli.Flags) error {
		return nil
	}

	cmd.SetAction(fakeAction)
	cmd.SetDescription("first action")
	cmd.SetFlags(cli.StringFlag{
		Name:     "arg",
		Usage:    "this is a test arg",
		Required: true,
		Value:    "default",
	})
	cmd.SetSubCommand("second")

	require.Len(t, builder.commands, 1)
	require.Len(t, builder.flags, 0)

	cmd2 := builder.commands[0]
	require.Len(t, cmd2.flags, 1)
	require.Len(t, cmd2.subcommands, 1)
}

func TestBuildFlags(t *testing.T) {
	in := []cli.Flag{
		cli.StringFlag{
			Name:     "name1",
			Usage:    "usage1",
			Required: true,
			Value:    "value1",
		},
		cli.StringSliceFlag{
			Name:     "name2",
			Usage:    "usage2",
			Required: true,
			Value:    []string{},
		},
		cli.DurationFlag{
			Name:     "name3",
			Usage:    "usage3",
			Required: true,
			Value:    time.Minute,
		},
		cli.IntFlag{
			Name:     "name4",
			Usage:    "usage4",
			Required: true,
			Value:    1,
		},
		cli.BoolFlag{
			Name:     "name5",
			Usage:    "usage5",
			Required: true,
			Value:    true,
		},
	}

	out := buildFlags(in)
	require.Len(t, out, 5)

	require.Equal(t, "name1", out[0].Names()[0])
	require.Equal(t, "name2", out[1].Names()[0])
	require.Equal(t, "name3", out[2].Names()[0])
	require.Equal(t, "name4", out[3].Names()[0])
	require.Equal(t, "name5", out[4].Names()[0])
}

func TestBuildFlags_Panic(t *testing.T) {
	defer func() {
		r := recover()
		require.Equal(t, "flag type '<nil>' not supported", r)
	}()

	buildFlags([]cli.Flag{nil})
}

func TestMakeAction(t *testing.T) {
	res := makeAction(nil)
	require.Nil(t, res)

	isCalled := false
	fakeAction := func(flags cli.Flags) error {
		require.Nil(t, flags)
		isCalled = true
		return nil
	}

	res = makeAction(fakeAction)
	require.NotNil(t, res)

	out := res(nil)
	require.NoError(t, out)
	require.True(t, isCalled)
}
