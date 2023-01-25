package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/dkg/pedersen"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

func TestMinimal_SetCommands(t *testing.T) {
	minimal := NewMinimal()

	b := node.NewBuilder()
	minimal.SetCommands(b)
}

func TestMinimal_OnStart(t *testing.T) {
	minimal := NewMinimal()

	inj := newInjector(fake.Mino{})
	err := minimal.OnStart(nil, inj)
	require.NoError(t, err)

	require.Len(t, inj.(*fakeInjector).history, 1)
	require.IsType(t, &pedersen.Pedersen{}, inj.(*fakeInjector).history[0])

	err = minimal.OnStart(nil, newBadInjector())
	require.EqualError(t, err, fake.Err("failed to resolve mino"))
}

func TestMinimal_OnStop(t *testing.T) {
	minimal := NewMinimal()

	err := minimal.OnStop(node.NewInjector())
	require.NoError(t, err)
}

// -----------------------------------------------------------------------------
// Utility functions

func newInjector(mino mino.Mino) node.Injector {
	return &fakeInjector{
		mino: mino,
	}
}

func newBadInjector() node.Injector {
	return &fakeInjector{
		isBad: true,
	}
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
		return fake.GetError()
	}

	switch msg := el.(type) {
	case *mino.Mino:
		if i.mino == nil {
			return fake.GetError()
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
