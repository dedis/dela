package node

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"sync"
	"time"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/cli"
	"golang.org/x/xerrors"
)

// SocketClient opens a connection to a unix socket daemon to send commands.
//
// - implements node.Client
type socketClient struct {
	socketpath string
	out        io.Writer
}

// Send implements node.Client. It opens a connection and sends the data to the
// daemon. It writes the result of the command to the output.
func (c socketClient) Send(data []byte) error {
	conn, err := net.Dial("unix", c.socketpath)
	if err != nil {
		return xerrors.Errorf("couldn't open connection: %v", err)
	}

	defer conn.Close()

	_, err = conn.Write(data)
	if err != nil {
		return xerrors.Errorf("couldn't write to daemon: %v", err)
	}

	_, err = io.Copy(c.out, conn)
	if err != nil {
		return xerrors.Errorf("couldn't read output: %v", err)
	}

	return nil
}

// SocketDaemon is a daemon using UNIX socket. This allows the permissions to be
// managed by the filesystem. A user must have read/write access to send a
// command to the daemon.
//
// - implements node.Daemon
type socketDaemon struct {
	sync.WaitGroup
	socketpath  string
	injector    Injector
	actions     *actionMap
	closing     chan struct{}
	readTimeout time.Duration
}

// Listen implements node.Daemon. It starts the daemon by creating the unix
// socket file to the path.
func (d *socketDaemon) Listen() error {
	socket, err := net.Listen("unix", d.socketpath)
	if err != nil {
		return xerrors.Errorf("couldn't bind socket: %v", err)
	}

	d.Add(2)

	go func() {
		defer d.Done()

		<-d.closing
		socket.Close()
	}()

	go func() {
		defer d.Done()

		for {
			fd, err := socket.Accept()
			if err != nil {
				select {
				case <-d.closing:
				default:
					dela.Logger.Err(err).Msg("daemon closed unexpectedly")
				}
				return
			}

			go d.handleConn(fd)
		}
	}()

	return nil
}

func (d *socketDaemon) handleConn(conn net.Conn) {
	defer conn.Close()

	// Read the first two bytes that will be converted into the action ID.
	buffer := make([]byte, 2)

	conn.SetReadDeadline(time.Now().Add(d.readTimeout))

	_, err := conn.Read(buffer)
	if err != nil {
		d.sendError(conn, xerrors.Errorf("stream corrupted: %v", err))
		return
	}

	dec := json.NewDecoder(conn)

	fset := make(FlagSet)
	err = dec.Decode(&fset)
	if err != nil {
		d.sendError(conn, xerrors.Errorf("failed to decode flags: %v", err))
		return
	}

	id := binary.LittleEndian.Uint16(buffer)
	action := d.actions.Get(id)

	if action == nil {
		d.sendError(conn, xerrors.Errorf("unknown command '%d'", id))
		return
	}

	actx := Context{
		Injector: d.injector,
		Flags:    fset,
		Out:      conn,
	}

	err = action.Execute(actx)
	if err != nil {
		d.sendError(conn, xerrors.Errorf("command error: %v", err))
		return
	}
}

func (d *socketDaemon) sendError(conn net.Conn, err error) {
	fmt.Fprintf(conn, "[ERROR] %v\n", err)
}

// Close implements node.Daemon. It closes the daemon and waits for the go
// routines to close.
func (d *socketDaemon) Close() error {
	close(d.closing)
	d.Wait()

	return nil
}

// SocketFactory provides primitives to create a daemon and clients from a CLI
// context.
//
// - implements node.DaemonFactory
type socketFactory struct {
	injector Injector
	actions  *actionMap
	out      io.Writer
}

// ClientFromContext implements node.DaemonFactory. It creates a client based on
// the flags of the context.
func (f socketFactory) ClientFromContext(ctx cli.Flags) (Client, error) {
	client := socketClient{
		socketpath: f.getSocketPath(ctx),
		out:        f.out,
	}

	return client, nil
}

// DaemonFromContext implements node.DaemonFactory. It creates a daemon based on
// the flags of the context.
func (f socketFactory) DaemonFromContext(ctx cli.Flags) (Daemon, error) {
	daemon := &socketDaemon{
		socketpath:  f.getSocketPath(ctx),
		injector:    f.injector,
		actions:     f.actions,
		closing:     make(chan struct{}),
		readTimeout: time.Second,
	}

	return daemon, nil
}

func (f socketFactory) getSocketPath(ctx cli.Flags) string {
	return filepath.Join(ctx.Path("config"), "daemon.sock")
}
