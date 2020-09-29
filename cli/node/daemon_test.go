package node

import (
	"bytes"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/internal/testing/fake"
	"golang.org/x/xerrors"
)

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

	// Windows only allows opening one socket per address, this is why we use
	// another one.
	path = filepath.Join(os.TempDir(), "daemon2.sock")
	client.socketpath = path

	listen(t, path, true)
	in := make([]byte, 256*1000) // fill the buffer
	err = client.Send(in)
	require.Error(t, err)

	if runtime.GOOS == "linux" {
		require.EqualError(t, err,
			"couldn't write to daemon: write unix @->/tmp/daemon2.sock: write: broken pipe")
	}
}

func TestSocketDaemon_Listen(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "dela-test-")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	actions := &actionMap{}
	actions.Set(fakeAction{})                         // id 0
	actions.Set(fakeAction{err: xerrors.New("oops")}) // id 1

	daemon := &socketDaemon{
		socketpath:  filepath.Join(dir, "daemon.sock"),
		actions:     actions,
		closing:     make(chan struct{}),
		readTimeout: 50 * time.Millisecond,
	}

	err = daemon.Listen()
	require.NoError(t, err)

	defer daemon.Close()

	out := new(bytes.Buffer)
	client := socketClient{
		socketpath: daemon.socketpath,
		out:        out,
	}

	err = client.Send(append([]byte{0x0, 0x0}, []byte("{}")...))
	require.NoError(t, err)
	require.Equal(t, "deadbeef", out.String())

	out.Reset()
	err = client.Send(append([]byte{0x1, 0x0}, []byte("{}")...))
	require.NoError(t, err)
	require.Equal(t, "[ERROR] command error: oops\n", out.String())

	out.Reset()
	err = client.Send(append([]byte{0x2, 0x0}, []byte("{}")...))
	require.NoError(t, err)
	require.Equal(t, "[ERROR] unknown command '2'\n", out.String())

	out.Reset()
	err = client.Send([]byte{0x0, 0x0, 0x0})
	require.NoError(t, err)
	require.Contains(t, out.String(), "[ERROR] failed to decode flags: ")

	out.Reset()
	err = client.Send([]byte{})
	require.NoError(t, err)
	require.Contains(t, out.String(), "[ERROR] stream corrupted: ")

	// the rest is not concerned by windows that actually allows the creation of
	// root files and folders
	if runtime.GOOS == "windows" {
		t.Skip()
	}

	daemon.socketpath = "/test.sock"
	err = daemon.Listen()
	require.Error(t, err)
	require.Regexp(t, "^couldn't bind socket: listen unix /test.sock: bind:", err)
}

func TestSocketFactory_ClientFromContext(t *testing.T) {
	factory := socketFactory{}

	client, err := factory.ClientFromContext(fakeContext{path: "cfgdir"})
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, filepath.Join("cfgdir", "daemon.sock"),
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

type fakeInitializer struct {
	err error
}

func (c fakeInitializer) SetCommands(Builder) {}

func (c fakeInitializer) Inject(cli.Flags, Injector) error {
	return c.err
}

type fakeClient struct {
	err   error
	calls *fake.Call
}

func (c fakeClient) Send(data []byte) error {
	c.calls.Add(data)
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
	calls     *fake.Call
}

func (f fakeFactory) ClientFromContext(cli.Flags) (Client, error) {
	return fakeClient{err: f.errClient, calls: f.calls}, f.err
}

func (f fakeFactory) DaemonFromContext(cli.Flags) (Daemon, error) {
	return fakeDaemon{err: f.errDaemon}, f.err
}

type fakeAction struct {
	err error
}

func (a fakeAction) Execute(req Context) error {
	if a.err != nil {
		return a.err
	}

	req.Out.Write([]byte("deadbeef"))
	return nil
}

type fakeContext struct {
	cli.Flags
	path string
}

func (ctx fakeContext) Path(name string) string {
	return ctx.path
}
