package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/execution/native"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestSetCommands(t *testing.T) {
	ctrl := NewController()

	call := &fake.Call{}
	ctrl.SetCommands(fakeBuilder{call: call})

	require.Equal(t, call.Len(), 7)
}

func TestOnStart(t *testing.T) {
	ctrl := NewController()

	injector := node.NewInjector()
	err := ctrl.OnStart(node.FlagSet{}, injector)
	require.EqualError(t, err, "failed to resolve access service: couldn't find dependency for 'access.Service'")

	access := fakeAccess{}
	injector.Inject(&access)

	err = ctrl.OnStart(node.FlagSet{}, injector)
	require.EqualError(t, err, "failed to resolve native service: couldn't find dependency for '*native.Service'")

	native := native.NewExecution()
	injector.Inject(native)

	oldStore := newStore
	newStore = func(path string) (accessStore, error) {
		return nil, fake.GetError()
	}

	err = ctrl.OnStart(node.FlagSet{}, injector)
	require.EqualError(t, err, fake.Err("failed to create access store"))

	newStore = oldStore

	err = ctrl.OnStart(node.FlagSet{}, injector)
	require.NoError(t, err)
}

func TestOnStop(t *testing.T) {
	ctrl := NewController()

	err := ctrl.OnStop(nil)
	require.NoError(t, err)
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

type fakeAccess struct {
	access.Service

	err error
}

func (a fakeAccess) Grant(store store.Snapshot, creds access.Credential, idents ...access.Identity) error {
	return a.err
}
