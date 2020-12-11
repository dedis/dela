package urfave

import (
	"io/ioutil"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	ucli "github.com/urfave/cli/v2"
	"go.dedis.ch/dela/cli"
)

func TestBuild(t *testing.T) {
	builder := NewBuilder("test", nil)
	app := builder.Build().(*ucli.App)

	app.Writer = ioutil.Discard

	require.Equal(t, "test", app.Name)

	err := app.Run([]string{"test"})
	require.NoError(t, err)
}

func TestSetCommand(t *testing.T) {
	builder := NewBuilder("test", nil)

	builder.SetCommand("first")
	builder.SetCommand("second")

	app := builder.Build().(*ucli.App)

	// urfave adds by default the -help command. This is why we expect 3.
	require.Len(t, app.Commands, 3)

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

	cmd2 := builder.commands["first"]
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
	}

	out := buildFlags(in)
	require.Len(t, out, 4)
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

	fakeAction := func(flags cli.Flags) error {
		return nil
	}

	res = makeAction(fakeAction)
	require.NotNil(t, res)

	out := res(&ucli.Context{})
	require.NoError(t, out)
}
