package controller

import (
	"testing"
	"time"

	"go.dedis.ch/dela/mino/minows"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/testing/fake"
)

func TestController_OnStart(t *testing.T) {
	flags, inj, ctrl, stop := setUp(t, "/ip4/0.0.0.0/tcp/8000/ws",
		"/dns4/p2p-1.c4dt.dela.org/tcp/443/wss")
	defer stop()

	err := ctrl.OnStart(flags, inj)
	require.NoError(t, err)
	var m *minows.Minows
	err = inj.Resolve(&m)
	require.NoError(t, err)
}

func TestController_OptionalPublic(t *testing.T) {
	flags, inj, ctrl, stop := setUp(t, "/ip4/0.0.0.0/tcp/8000/ws", "")
	defer stop()

	err := ctrl.OnStart(flags, inj)
	require.NoError(t, err)
	var m *minows.Minows
	err = inj.Resolve(&m)
	require.NoError(t, err)
}

func TestController_InvalidListen(t *testing.T) {
	flags, inj, ctrl, _ := setUp(t, "invalid",
		"/dns4/p2p-1.c4dt.dela.org/tcp/443/wss")

	err := ctrl.OnStart(flags, inj)
	require.Error(t, err)
}

func TestController_InvalidPublic(t *testing.T) {
	flags, inj, ctrl, _ := setUp(t, "/ip4/0.0.0.0/tcp/8000/ws",
		"invalid")

	err := ctrl.OnStart(flags, inj)
	require.Error(t, err)
}

func TestController_OnStop(t *testing.T) {
	flags, inj, ctrl, _ := setUp(t, "/ip4/0.0.0.0/tcp/8000/ws",
		"/dns4/p2p-1.c4dt.dela.org/tcp/443/wss")
	err := ctrl.OnStart(flags, inj)
	require.NoError(t, err)

	err = ctrl.OnStop(inj)
	require.NoError(t, err)
}

func TestController_MissingDependency(t *testing.T) {
	flags, inj, ctrl, _ := setUp(t, "/ip4/0.0.0.0/tcp/8000/ws",
		"/dns4/p2p-1.c4dt.dela.org/tcp/443/wss")
	err := ctrl.OnStart(flags, inj)
	require.NoError(t, err)

	err = ctrl.OnStop(node.NewInjector())
	require.Error(t, err)
}

func mustCreateController(t *testing.T, inj node.Injector) (node.Initializer, func()) {
	ctrl := NewController()
	stop := func() {
		require.NoError(t, ctrl.OnStop(inj))
	}
	return ctrl, stop
}

func setUp(t *testing.T, listen string, public string) (
	cli.Flags, node.Injector, node.Initializer, func(),
) {
	flags := new(mockFlags)
	flags.On("String", "listen").Return(listen)
	flags.On("String", "public").Return(public)
	inj := node.NewInjector()
	inj.Inject(fake.NewInMemoryDB())

	ctrl, stop := mustCreateController(t, inj)

	return flags, inj, ctrl, stop
}

// mockFlags
// - implements cli.Flags
type mockFlags struct {
	mock.Mock
}

func (m *mockFlags) String(name string) string {
	args := m.Called(name)
	return args.String(0)
}

func (m *mockFlags) StringSlice(name string) []string {
	args := m.Called(name)
	return args.Get(0).([]string)
}

func (m *mockFlags) Duration(name string) time.Duration {
	args := m.Called(name)
	return args.Get(0).(time.Duration)
}

func (m *mockFlags) Path(name string) string {
	args := m.Called(name)
	return args.String(0)
}

func (m *mockFlags) Int(name string) int {
	args := m.Called(name)
	return args.Int(0)
}

func (m *mockFlags) Bool(name string) bool {
	args := m.Called(name)
	return args.Bool(0)
}
