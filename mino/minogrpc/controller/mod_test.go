package controller

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino/minogrpc"
)

func TestMinimal_Build(t *testing.T) {
	minimal := NewMinimal()

	call := &fake.Call{}
	minimal.SetCommands(fakeBuilder{call: call})

	require.Equal(t, 17, call.Len())
}

func TestMinimal_OnStart(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "minogrpc")
	require.NoError(t, err)

	db, err := kv.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	minimal := NewMinimal()

	injector := node.NewInjector()
	injector.Inject(db)

	err = minimal.OnStart(fakeContext{path: dir}, injector)
	require.NoError(t, err)

	var m *minogrpc.Minogrpc
	err = injector.Resolve(&m)
	require.NoError(t, err)
	require.NoError(t, m.GracefulStop())

	err = minimal.OnStart(fakeContext{num: 100000}, injector)
	require.EqualError(t, err, "invalid port value 100000")
}

func TestMinimal_OnStop(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "minogrpc")
	require.NoError(t, err)

	db, err := kv.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	minimal := NewMinimal()

	injector := node.NewInjector()
	injector.Inject(db)

	err = minimal.OnStart(fakeContext{path: dir}, injector)
	require.NoError(t, err)

	err = minimal.OnStop(injector)
	require.NoError(t, err)

	err = minimal.OnStop(node.NewInjector())
	require.EqualError(t, err, "injector: couldn't find dependency for 'controller.StoppableMino'")

	injector = node.NewInjector()
	injector.Inject(badMino{})
	err = minimal.OnStop(injector)
	require.EqualError(t, err, fake.Err("while stopping mino"))
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

type badMino struct {
	StoppableMino
}

func (badMino) GracefulStop() error {
	return fake.GetError()
}
