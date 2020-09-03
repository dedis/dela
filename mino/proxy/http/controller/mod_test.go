package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/proxy/http"
	"golang.org/x/xerrors"
)

func TestMinimal_SetCommands(t *testing.T) {
	minimal := NewMinimal()
	builder := &fakeBuilder{}
	minimal.SetCommands(builder)

	require.Len(t, builder.Startflags, 1)
	require.IsType(t, cli.StringFlag{}, builder.Startflags[0])
	require.Equal(t, "clientaddr", builder.Startflags[0].(cli.StringFlag).Name)
}

func TestMinimal_Inject(t *testing.T) {
	minimal := NewMinimal()

	inj := newInjector()
	err := minimal.Inject(fakeFlags{
		Strings: map[string]string{
			// we use a different port here because the server runing from the
			// previous test might not be fully stop at the time this test
			// starts.
			"clientaddr": "127.0.0.1:2501",
		},
	}, inj)
	require.NoError(t, err)

	require.Len(t, inj.(*fakeInjector).history, 1)
	require.IsType(t, &http.HTTP{}, inj.(*fakeInjector).history[0])

	aa := inj.(*fakeInjector).history[0]
	aa.(*http.HTTP).Stop()
}

// -----------------------------------------------------------------------------
// Utility functions

// fakeBuilder is a fake builders
//
// - implements node.Builder
type fakeBuilder struct {
	Startflags []cli.Flag
}

// SetCommand implements node.Builder
func (f fakeBuilder) SetCommand(name string) cli.CommandBuilder {
	return nil
}

// SetStartFlags implements node.Builder
func (f *fakeBuilder) SetStartFlags(flags ...cli.Flag) {
	f.Startflags = flags
}

// MakeNodeAction implements node.Builder
func (f fakeBuilder) MakeAction(_ node.ActionTemplate) cli.Action {
	return nil
}

func newInjector() node.Injector {
	return &fakeInjector{}
}

// fakeInjector is a fake injector
//
// - implements node.Injector
type fakeInjector struct {
	isBad   bool
	mino    mino.Mino
	history []interface{}
}

// Resolve implements node.Injector
func (i fakeInjector) Resolve(el interface{}) error {
	if i.isBad {
		return xerrors.New("oops")
	}

	switch msg := el.(type) {
	case *mino.Mino:
		if i.mino == nil {
			return xerrors.New("oops")
		}
		*msg = i.mino
	default:
		return xerrors.Errorf("unkown message '%T", msg)
	}

	return nil
}

// Inject implements node.Injector
func (i *fakeInjector) Inject(v interface{}) {
	if i.history == nil {
		i.history = make([]interface{}, 0)
	}
	i.history = append(i.history, v)
}

// fakeFlags is a fake flags
//
// - implements cli.Flags
type fakeFlags struct {
	cli.Flags

	Strings map[string]string
	Ints    map[string]int
}

// String implements cli.Flags
func (f fakeFlags) String(name string) string {
	return f.Strings[name]
}

// Int implements cli.Flags
func (f fakeFlags) Int(name string) int {
	return f.Ints[name]
}
