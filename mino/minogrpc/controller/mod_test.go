package controller

import (
	"crypto/elliptic"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino/minogrpc"
)

func TestMiniController_Build(t *testing.T) {
	ctrl := NewController()

	call := &fake.Call{}
	ctrl.SetCommands(fakeBuilder{call: call})

	require.Equal(t, 22, call.Len())
}

func TestMiniController_OnStart(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "minogrpc")
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	db, err := kv.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)

	ctrl := NewController().(miniController)

	injector := node.NewInjector()
	injector.Inject(db)

	str := map[string]string{"routing": "flat"}

	err = ctrl.OnStart(fakeContext{path: dir, str: str}, injector)
	require.NoError(t, err)

	str = map[string]string{"routing": "tree"}

	err = ctrl.OnStart(fakeContext{path: dir, str: str}, injector)
	require.NoError(t, err)

	var m *minogrpc.Minogrpc
	err = injector.Resolve(&m)
	require.NoError(t, err)
	require.NoError(t, m.GracefulStop())
}

func TestMiniController_InvalidAddr_OnStart(t *testing.T) {
	ctrl := NewController()

	str := map[string]string{"listen": ":xxx"}

	err := ctrl.OnStart(fakeContext{str: str}, node.NewInjector())
	require.EqualError(t, err, "failed to parse listen URL: parse \":xxx\": missing protocol scheme")
}

func TestMiniController_OverlayFailed_OnStart(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "minogrpc")
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	db, err := kv.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)

	ctrl := NewController().(miniController)

	injector := node.NewInjector()
	injector.Inject(db)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	defer listener.Close()

	// The address is correct but it will yield an error because it is already
	// used.

	str := map[string]string{"listen": "tcp://" + listener.Addr().String(), "routing": "flat"}

	err = ctrl.OnStart(fakeContext{path: dir, str: str}, injector)
	require.True(t, strings.HasPrefix(err.Error(), "couldn't make overlay: failed to bind"), err.Error())
}

func TestMiniController_MissingDB_OnStart(t *testing.T) {
	ctrl := NewController()

	str := map[string]string{"routing": "flat"}

	err := ctrl.OnStart(fakeContext{str: str}, node.NewInjector())
	require.EqualError(t, err, "injector: couldn't find dependency for 'kv.DB'")
}

func TestMiniController_UnknownRouting_OnStart(t *testing.T) {
	ctrl := NewController()

	str := map[string]string{"routing": "fake"}

	err := ctrl.OnStart(fakeContext{str: str}, node.NewInjector())
	require.EqualError(t, err, "unknown routing: fake")
}

func TestMiniController_FailGenerateKey_OnStart(t *testing.T) {
	ctrl := NewController().(miniController)
	ctrl.random = badReader{}

	inj := node.NewInjector()
	inj.Inject(fake.NewInMemoryDB())

	str := map[string]string{"routing": "flat"}

	err := ctrl.OnStart(fakeContext{str: str}, inj)
	require.EqualError(t, err,
		fake.Err("cert private key: while loading: generator failed: ecdsa"))
}

func TestMiniController_FailMarshalKey_OnStart(t *testing.T) {
	ctrl := NewController().(miniController)
	ctrl.curve = badCurve{Curve: elliptic.P224()}

	inj := node.NewInjector()
	inj.Inject(fake.NewInMemoryDB())

	str := map[string]string{"routing": "flat"}

	err := ctrl.OnStart(fakeContext{str: str}, inj)
	require.EqualError(t, err,
		"cert private key: while loading: generator failed: while marshaling: x509: unknown elliptic curve")
}

func TestMiniController_FailParseKey_OnStart(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "dela")
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	ctrl := NewController().(miniController)

	inj := node.NewInjector()
	inj.Inject(fake.NewInMemoryDB())

	file, err := os.Create(filepath.Join(dir, certKeyName))
	require.NoError(t, err)

	defer file.Close()

	str := map[string]string{"routing": "flat"}

	err = ctrl.OnStart(fakeContext{path: dir, str: str}, inj)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cert private key: while parsing: x509: ")
}

func TestMiniController_FailedTCPResolve_OnStart(t *testing.T) {
	ctrl := NewController()

	str := map[string]string{"listen": "yyy:xxx"}

	err := ctrl.OnStart(fakeContext{str: str}, node.NewInjector())
	require.EqualError(t, err, "failed to resolve tcp address: unknown network yyy")
}

func TestMiniController_FailedPublicParse_OnStart(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "minogrpc")
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	db, err := kv.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)

	ctrl := NewController().(miniController)

	injector := node.NewInjector()
	injector.Inject(db)

	// The address is correct but it will yield an error because it is already
	// used.

	str := map[string]string{"listen": "tcp://1.2.3.4:0", "public": ":xxx", "routing": "flat"}

	err = ctrl.OnStart(fakeContext{path: dir, str: str}, injector)
	require.EqualError(t, err, `failed to parse public: parse ":xxx": missing protocol scheme`)
}

func TestMiniController_OnStop(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "minogrpc")
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	db, err := kv.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)

	ctrl := NewController()

	injector := node.NewInjector()
	injector.Inject(db)

	str := map[string]string{"routing": "flat"}

	err = ctrl.OnStart(fakeContext{path: dir, str: str}, injector)
	require.NoError(t, err)

	err = ctrl.OnStop(injector)
	require.NoError(t, err)
}

func TestMiniController_MissingMino_OnStop(t *testing.T) {
	ctrl := NewController()

	err := ctrl.OnStop(node.NewInjector())
	require.EqualError(t, err, "injector: couldn't find dependency for 'controller.StoppableMino'")
}

func TestMiniController_FailStopMino_OnStop(t *testing.T) {
	ctrl := NewController()

	inj := node.NewInjector()
	inj.Inject(badMino{})

	err := ctrl.OnStop(inj)
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

type badReader struct{}

func (badReader) Read([]byte) (int, error) {
	return 0, fake.GetError()
}

type badCurve struct {
	elliptic.Curve
}
