package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestMiniController_Build(t *testing.T) {
	ctrl := NewController()

	call := &fake.Call{}
	ctrl.SetCommands(fakeBuilder{call: call})

	require.Equal(t, 7, call.Len())
	require.Equal(t, "pool", call.Get(0, 0))
	require.Equal(t, "interact with the pool", call.Get(1, 0))
	require.Equal(t, "add", call.Get(2, 0))
	require.Equal(t, "add a transaction to the pool", call.Get(3, 0))
	require.Len(t, call.Get(4, 0), 3)
	require.IsType(t, &addAction{}, call.Get(5, 0))
	require.Nil(t, call.Get(6, 0)) // our fake MakeAction() returns nil
}

func TestMiniController_OnStart(t *testing.T) {
	res := NewController().OnStart(node.FlagSet{}, nil)
	require.Nil(t, res)
}

func TestMiniController_OnStop(t *testing.T) {
	res := NewController().OnStop(nil)
	require.Nil(t, res)
}

func TestClient(t *testing.T) {
	c := client{}

	n, err := c.GetNonce(nil)
	require.NoError(t, err)
	require.Equal(t, uint64(0), n)

	n, err = c.GetNonce(nil)
	require.NoError(t, err)
	require.Equal(t, uint64(1), n)
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeCommandBuilder struct {
	call *fake.Call
}

func (b fakeCommandBuilder) SetSubCommand(name string) cli.CommandBuilder {
	b.call.Add(name)
	return b
}

func (b fakeCommandBuilder) SetDescription(value string) {
	b.call.Add(value)
}

func (b fakeCommandBuilder) SetFlags(flags ...cli.Flag) {
	b.call.Add(flags)
}

func (b fakeCommandBuilder) SetAction(a cli.Action) {
	b.call.Add(a)
}

type fakeBuilder struct {
	call *fake.Call
}

func (b fakeBuilder) SetCommand(name string) cli.CommandBuilder {
	b.call.Add(name)
	return fakeCommandBuilder(b)
}

func (b fakeBuilder) SetStartFlags(flags ...cli.Flag) {
	b.call.Add(flags)
}

func (b fakeBuilder) MakeAction(tmpl node.ActionTemplate) cli.Action {
	b.call.Add(tmpl)
	return nil
}
