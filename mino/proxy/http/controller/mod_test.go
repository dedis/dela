package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino/proxy/http"
)

func TestMinimal_SetCommands(t *testing.T) {
	minimal := NewController()
	call := fake.Call{}
	builder := &fakeBuilder{call: &call}
	minimal.SetCommands(builder)

	require.Equal(t, call.Len(), 11)
}

func TestMinimal_OnStart(t *testing.T) {
	minimal := NewController()

	err := minimal.OnStart(nil, nil)
	require.NoError(t, err)
}

func TestMinimal_OnStop(t *testing.T) {
	minimal := NewController()

	inj := node.NewInjector()

	proxy := http.NewHTTP("127.0.0.1:0")
	go proxy.Listen()

	inj.Inject(proxy)

	err := minimal.OnStop(inj)
	require.NoError(t, err)

	err = minimal.OnStop(node.NewInjector())
	require.NoError(t, err)
}

// -----------------------------------------------------------------------------
// Utility functions

// fakeCommandBuilder is a fake command builder
//
// - implements cli.CommandBuilder
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

// fakeBuilder is a fake builders
//
// - implements node.Builder
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
