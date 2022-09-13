package node

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/internal/testing/fake"
	"golang.org/x/xerrors"
)

func TestSocketClient_Send(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "dela")
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	out := new(bytes.Buffer)

	client := socketClient{
		socketpath: filepath.Join(dir, "daemon.sock"),
		out:        out,
		dialFn:     net.DialTimeout,
	}

	listen(t, client.socketpath)

	err = client.Send([]byte("deadbeef"))
	require.NoError(t, err)
	require.Equal(t, "deadbeef\n", out.String())
}

func TestSocketClient_FailDial_Send(t *testing.T) {
	client := socketClient{
		socketpath: "",
		dialFn: func(network, addr string, timeout time.Duration) (net.Conn, error) {
			return nil, fake.GetError()
		},
	}

	err := client.Send(nil)
	require.EqualError(t, err, fake.Err("couldn't open connection"))
}

func TestSocketClient_BadOutConn_Send(t *testing.T) {
	client := socketClient{
		dialFn: func(network, addr string, timeout time.Duration) (net.Conn, error) {
			return badConn{}, nil
		},
	}

	err := client.Send([]byte{1, 2, 3})
	require.EqualError(t, err, fake.Err("couldn't write to daemon"))
}

func TestSocketClient_BadInConn_Send(t *testing.T) {
	client := socketClient{
		dialFn: func(network, addr string, timeout time.Duration) (net.Conn, error) {
			return badConn{counter: fake.NewCounter(1)}, nil
		},
	}

	err := client.Send([]byte{})
	require.EqualError(t, err, fake.Err("fail to decode event"))
}

func TestSocketDaemon_Listen(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "dela")
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	fset := make(FlagSet)
	fset["1"] = 1

	buf, err := json.Marshal(&fset)
	require.NoError(t, err)

	actions := &actionMap{}
	actions.Set(fakeAction{
		intFlags: map[string]int{"1": 1},
	})                                            // id 0
	actions.Set(fakeAction{err: fake.GetError()}) // id 1

	daemon := &socketDaemon{
		socketpath:  filepath.Join(dir, "daemon.sock"),
		actions:     actions,
		closing:     make(chan struct{}),
		readTimeout: 50 * time.Millisecond,
		listenFn:    net.Listen,
	}

	err = daemon.Listen()
	require.NoError(t, err)

	defer daemon.Close()

	out := new(bytes.Buffer)
	client := socketClient{
		socketpath:  daemon.socketpath,
		out:         out,
		dialTimeout: time.Second,
		dialFn:      net.DialTimeout,
	}

	err = client.Send(append([]byte{0x0, 0x0}, buf...))
	require.NoError(t, err)
	require.Equal(t, "deadbeef\n", out.String())

	err = client.Send(append([]byte{0x1, 0x0}, []byte("{}")...))
	require.EqualError(t, err, fake.Err("command error"))

	err = client.Send(append([]byte{0x2, 0x0}, []byte("{}")...))
	require.EqualError(t, err, "unknown command '2'")

	err = client.Send([]byte{0x0, 0x0, 0x0})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to decode flags")

	err = client.Send([]byte{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "stream corrupted: ")
}

func TestSocketDaemon_ConnectivityTest_Listen(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "dela")
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	daemon := &socketDaemon{
		socketpath:  filepath.Join(dir, "daemon.sock"),
		actions:     &actionMap{},
		closing:     make(chan struct{}),
		readTimeout: 50 * time.Millisecond,
		listenFn:    net.Listen,
	}

	err = daemon.Listen()
	require.NoError(t, err)

	defer daemon.Close()

	conn, err := net.DialTimeout("unix", daemon.socketpath, 1*time.Second)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}

func TestSocketDaemon_FailBindSocket_Listen(t *testing.T) {
	daemon := &socketDaemon{
		listenFn: func(network, addr string) (net.Listener, error) {
			return nil, fake.GetError()
		},
	}

	err := daemon.Listen()
	require.EqualError(t, err, fake.Err("couldn't bind socket"))
}

func TestSocketDaemon_ConnClosedFromClient_HandleConn(t *testing.T) {
	logger, check := fake.CheckLog("connection to daemon has error")

	daemon := &socketDaemon{
		logger:      logger,
		actions:     &actionMap{},
		closing:     make(chan struct{}),
		readTimeout: 50 * time.Millisecond,
	}

	daemon.handleConn(badConn{})

	check(t)
}

func TestClientWriter_Write(t *testing.T) {
	buffer := new(bytes.Buffer)

	w := newClientWriter(buffer)

	n, err := w.Write([]byte("deadbeef"))
	require.NoError(t, err)
	require.Equal(t, 8, n)
}

func TestClientWriter_BadWriter_Write(t *testing.T) {
	w := newClientWriter(fake.NewBadHash())

	n, err := w.Write([]byte("deadbeef"))
	require.Equal(t, 0, n)
	require.EqualError(t, err, fake.Err("while packing data"))
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

func listen(t *testing.T, path string) {
	socket, err := net.Listen("unix", path)
	require.NoError(t, err)

	go func() {
		conn, err := socket.Accept()
		require.NoError(t, err)

		defer conn.Close()
		defer socket.Close()

		buffer := make([]byte, 100)
		n, err := conn.Read(buffer)
		require.NoError(t, err)

		enc := json.NewEncoder(conn)
		err = enc.Encode(event{Value: string(buffer[:n])})
		require.NoError(t, err)
	}()
}

type fakeInitializer struct {
	err     error
	errStop error
}

func (c fakeInitializer) SetCommands(Builder) {}

func (c fakeInitializer) OnStart(cli.Flags, Injector) error {
	return c.err
}

func (c fakeInitializer) OnStop(Injector) error {
	return c.errStop
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
	err      error
	intFlags map[string]int
}

func (a fakeAction) Execute(req Context) error {
	if a.err != nil {
		return a.err
	}

	if a.intFlags != nil {
		for k, v := range a.intFlags {
			if req.Flags.Int(k) != v {
				return xerrors.Errorf("missing flag %s", k)
			}
		}
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

type badConn struct {
	net.Conn

	counter *fake.Counter
}

func (conn badConn) Read(data []byte) (int, error) {
	if !conn.counter.Done() {
		conn.counter.Decrease()
		return len(data), nil
	}

	return 0, fake.GetError()
}

func (conn badConn) Write(data []byte) (int, error) {
	if !conn.counter.Done() {
		conn.counter.Decrease()
		return len(data), nil
	}

	return 0, fake.GetError()
}

func (badConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (badConn) Close() error {
	return nil
}
