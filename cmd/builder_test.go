package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

func TestCliBuilder_Command(t *testing.T) {
	builder := &cliBuilder{}

	cmd := builder.Command("test")
	require.NotNil(t, cmd)
	require.Len(t, builder.commands, 1)
}

func TestCliBuilder_Start(t *testing.T) {
	builder := &cliBuilder{}

	builder.Start(StringFlag{}, IntFlag{})
	require.Len(t, builder.startFlags, 2)
}

func TestCliBuilder_Build(t *testing.T) {
	builder := &cliBuilder{
		actions:       &actionMap{},
		daemonFactory: fakeFactory{},
	}

	builder.Command("test").
		Description("test description").
		Action(fakeAction{}).
		Flags(StringFlag{Name: "string-flag"}).
		Command("subtest").
		Description("subtest description").
		Flags(DurationFlag{}, IntFlag{})

	builder.Command("bad").
		Action(fakeAction{err: xerrors.New("oops")})

	commands := builder.build()
	require.Len(t, commands, 3)

	err := commands[0].Action(&cli.Context{})
	require.NoError(t, err)

	err = commands[1].Action(&cli.Context{})
	require.EqualError(t, err, "couldn't prepare action: oops")

	builder.daemonFactory = fakeFactory{err: xerrors.New("oops")}
	err = commands[0].Action(&cli.Context{})
	require.EqualError(t, err, "couldn't make client: oops")

	builder.daemonFactory = fakeFactory{errClient: xerrors.New("oops")}
	err = commands[0].Action(&cli.Context{})
	require.EqualError(t, err, "couldn't send action: oops")
}

func TestCliBuilder_UnknownType_BuildFlags(t *testing.T) {
	defer func() {
		r := recover()
		require.Equal(t, "flag type '<nil>' not supported", r)
	}()

	builder := &cliBuilder{}
	builder.Start((Flag)(nil))

	builder.buildFlags(builder.startFlags)
}

func TestFlags(t *testing.T) {
	StringFlag{}.Flag()
	DurationFlag{}.Flag()
	IntFlag{}.Flag()
}
