package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/execution/native"
	"go.dedis.ch/dela/core/store"
)

func TestSetCommands(t *testing.T) {
	ctrl := NewController()
	ctrl.SetCommands(nil)
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

type fakeAccess struct {
	access.Service

	err error
}

func (a fakeAccess) Grant(store.Snapshot, access.Credential, ...access.Identity) error {
	return a.err
}
