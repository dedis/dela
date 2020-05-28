package cmd

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"golang.org/x/xerrors"
)

func TestApp_Run(t *testing.T) {
	app := NewApp(fakeController{}).(cliApp)

	app.builder.sigs <- syscall.SIGTERM
	args := []string{"testapp", "--socket", "/tmp/dela/daemon.sock", "start"}

	err := app.Run(args)
	require.NoError(t, err)

	app.builder.daemonFactory = fakeFactory{err: xerrors.New("oops")}
	err = app.Run(args)
	require.EqualError(t, err,
		"failed to execute the command: couldn't make daemon: oops")

	app.builder.daemonFactory = fakeFactory{errDaemon: xerrors.New("oops")}
	err = app.Run(args)
	require.EqualError(t, err,
		"failed to execute the command: couldn't start the daemon: oops")

	app = NewApp(fakeController{err: xerrors.New("oops")}).(cliApp)
	err = app.Run(args)
	require.EqualError(t, err,
		"failed to execute the command: couldn't run the controller: oops")
}

func TestSocketClient_Send(t *testing.T) {
	path := filepath.Join(os.TempDir(), "daemon.sock")

	listen(t, path, false)

	out := new(bytes.Buffer)
	client := socketClient{
		socketpath: path,
		out:        out,
	}

	err := client.Send([]byte("deadbeef"))
	require.NoError(t, err)
	require.Equal(t, "deadbeef", out.String())

	client.socketpath = ""
	err = client.Send(nil)
	require.EqualError(t, err,
		"couldn't open connection: dial unix: missing address")

	listen(t, path, false)
	client.socketpath = path
	client.out = fake.NewBadHash()
	err = client.Send([]byte("deadbeef"))
	require.EqualError(t, err, "couldn't read output: fake error")

	listen(t, path, true)
	in := make([]byte, 256*1000) // fill the buffer
	err = client.Send(in)
	require.EqualError(t, err,
		"couldn't write to daemon: write unix @->/tmp/daemon.sock: write: broken pipe")
}

func TestSocketDaemon_Listen(t *testing.T) {
	path := filepath.Join(os.TempDir(), "dela", "daemon.sock")

	actions := &actionMap{}
	actions.Set(fakeAction{})                         // id 0
	actions.Set(fakeAction{err: xerrors.New("oops")}) // id 1

	daemon := &socketDaemon{
		socketpath: path,
		actions:    actions,
		closing:    make(chan struct{}),
	}

	err := daemon.Listen()
	require.NoError(t, err)

	defer daemon.Close()

	out := new(bytes.Buffer)
	client := socketClient{
		socketpath: path,
		out:        out,
	}

	err = client.Send([]byte{0x0})
	require.NoError(t, err)
	require.Equal(t, "deadbeef", out.String())

	out.Reset()
	err = client.Send([]byte{0x1})
	require.NoError(t, err)
	require.Equal(t, "[ERROR] command error: oops\n", out.String())

	out.Reset()
	err = client.Send([]byte{0x2})
	require.NoError(t, err)
	require.Equal(t, "[ERROR] unknown command '2'\n", out.String())

	daemon.socketpath = "/deadbeef/test.sock"
	err = daemon.Listen()
	require.EqualError(t, err,
		"couldn't make path: mkdir /deadbeef/: permission denied")

	daemon.socketpath = "/test.sock"
	err = daemon.Listen()
	require.EqualError(t, err,
		"couldn't bind socket: listen unix /test.sock: bind: permission denied")
}

func TestSocketFactory_ClientFromContext(t *testing.T) {
	factory := socketFactory{}

	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	client, err := factory.ClientFromContext(fakeContext{})
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, filepath.Join(homeDir, ".dela", "daemon.sock"),
		client.(socketClient).socketpath)

	require.NoError(t, syscall.Unsetenv("HOME"))
	defer syscall.Setenv("HOME", homeDir)

	client, err = factory.ClientFromContext(fakeContext{})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(os.TempDir(), "dela", "daemon.sock"),
		client.(socketClient).socketpath)
}

// -----------------------------------------------------------------------------
// Utility functions

func listen(t *testing.T, path string, quick bool) {
	socket, err := net.Listen("unix", path)
	require.NoError(t, err)

	go func() {
		conn, err := socket.Accept()
		require.NoError(t, err)

		defer conn.Close()
		defer socket.Close()

		if quick {
			return
		}

		buffer := make([]byte, 100)
		n, err := conn.Read(buffer)
		require.NoError(t, err)

		_, err = conn.Write(buffer[:n])
		require.NoError(t, err)
	}()
}

type fakeController struct {
	err error
}

func (c fakeController) Build(Builder) {}

func (c fakeController) Run(Context, Injector) error {
	return c.err
}

type fakeClient struct {
	err error
}

func (c fakeClient) Send([]byte) error {
	return c.err
}

type fakeDaemon struct {
	Daemon
	err error
}

func (d fakeDaemon) Listen() error {
	return d.err
}

type fakeFactory struct {
	DaemonFactory
	err       error
	errClient error
	errDaemon error
}

func (f fakeFactory) ClientFromContext(Context) (Client, error) {
	return fakeClient{err: f.errClient}, f.err
}

func (f fakeFactory) DaemonFromContext(Context) (Daemon, error) {
	return fakeDaemon{err: f.errDaemon}, f.err
}

type fakeAction struct {
	Action
	err error
}

func (a fakeAction) Prepare(Context) ([]byte, error) {
	return []byte{}, a.err
}

func (a fakeAction) Execute(req Request) error {
	if a.err != nil {
		return a.err
	}

	req.Out.Write([]byte("deadbeef"))
	return nil
}

type fakeContext struct {
	Context
	path string
}

func (ctx fakeContext) Path(name string) string {
	return ctx.path
}
