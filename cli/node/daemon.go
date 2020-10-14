// This file contains the implementation of a client and a daemon talking
// through a UNIX socket.
//
// Documentation Last Review: 13.10.2020
//

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

	"github.com/rs/zerolog"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/cli"
	"golang.org/x/xerrors"
)

const ioTimeout = 30 * time.Second

// event is the structure sent over the connection between the client and the
// daemon and vice-versa using a JSON encoding.
type event struct {
	Err   bool
	Value string
}

// SocketClient opens a connection to a unix socket daemon to send commands.
//
// - implements node.Client
type socketClient struct {
	socketpath  string
	out         io.Writer
	dialTimeout time.Duration
	dialFn      func(network, addr string, timeout time.Duration) (net.Conn, error)
}

// Send implements node.Client. It opens a connection and sends the data to the
// daemon. It writes the result of the command to the output.
func (c socketClient) Send(data []byte) error {
	conn, err := c.dialFn("unix", c.socketpath, c.dialTimeout)
	if err != nil {
		return xerrors.Errorf("couldn't open connection: %v", err)
	}

	defer conn.Close()

	_, err = conn.Write(data)
	if err != nil {
		return xerrors.Errorf("couldn't write to daemon: %v", err)
	}

	// The client will now wait for incoming messages from the daemon, either
	// results of the command, or an error if something goes wrong.
	dec := json.NewDecoder(conn)
	var evt event

	for {
		err = dec.Decode(&evt)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return xerrors.Errorf("fail to decode event: %v", err)
		}

		if evt.Err {
			return xerrors.New(evt.Value)
		}

		fmt.Fprintln(c.out, evt.Value)
	}
}

// SocketDaemon is a daemon using UNIX socket. This allows the permissions to be
// managed by the filesystem. A user must have read/write access to send a
// command to the daemon.
//
// - implements node.Daemon
type socketDaemon struct {
	sync.WaitGroup

	logger      zerolog.Logger
	socketpath  string
	injector    Injector
	actions     *actionMap
	closing     chan struct{}
	readTimeout time.Duration
	listenFn    func(network, addr string) (net.Listener, error)
}

// Listen implements node.Daemon. It starts the daemon by creating the unix
// socket file to the path.
func (d *socketDaemon) Listen() error {
	socket, err := d.listenFn("unix", d.socketpath)
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

	d.logger.Trace().Msg("daemon is handling a connection")

	// Read the first two bytes that will be converted into the action ID.
	buffer := make([]byte, 2)

	conn.SetReadDeadline(time.Now().Add(d.readTimeout))

	_, err := conn.Read(buffer)
	if err == io.EOF {
		// Connection closed upfront so it does not need further handling. This
		// happens for instance when testing the connectivity of the daemon.
		return
	}
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

	d.logger.Debug().
		Hex("command", buffer).
		Str("flags", fmt.Sprintf("%v", fset)).
		Msg("received command on the daemon")

	id := binary.LittleEndian.Uint16(buffer)
	action := d.actions.Get(id)

	if action == nil {
		d.sendError(conn, xerrors.Errorf("unknown command '%d'", id))
		return
	}

	actx := Context{
		Injector: d.injector,
		Flags:    fset,
		Out:      newClientWriter(conn),
	}

	err = action.Execute(actx)
	if err != nil {
		d.sendError(conn, xerrors.Errorf("command error: %v", err))
		return
	}
}

func (d *socketDaemon) sendError(conn net.Conn, err error) {
	enc := json.NewEncoder(conn)

	d.logger.Debug().Err(err).Msg("sending error to client")

	// The event contains an error which will make the command on the client
	// side fail with the value as the error message.
	err = enc.Encode(event{Err: true, Value: err.Error()})
	if err != nil {
		d.logger.Warn().Err(err).Msg("connection to daemon has error")
	}
}

// Close implements node.Daemon. It closes the daemon and waits for the go
// routines to close.
func (d *socketDaemon) Close() error {
	close(d.closing)
	d.Wait()

	return nil
}

// clientWriter is a wrapper around a socket connection that will write the data
// using a JSON message wrapper.
//
// - implements io.Writer
type clientWriter struct {
	enc *json.Encoder
}

func newClientWriter(w io.Writer) *clientWriter {
	return &clientWriter{
		enc: json.NewEncoder(w),
	}
}

// Write implements io.Writer. It wraps the data into a JSON message that is
// written to the parent writer. The number of written bytes returned
// corresponds to the input if successful.
func (w *clientWriter) Write(data []byte) (int, error) {
	err := w.enc.Encode(event{Value: string(data)})
	if err != nil {
		return 0, xerrors.Errorf("while packing data: %v", err)
	}

	return len(data), nil
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
		socketpath:  f.getSocketPath(ctx),
		out:         f.out,
		dialTimeout: ioTimeout,
		dialFn:      net.DialTimeout,
	}

	return client, nil
}

// DaemonFromContext implements node.DaemonFactory. It creates a daemon based on
// the flags of the context.
func (f socketFactory) DaemonFromContext(ctx cli.Flags) (Daemon, error) {
	socketpath := f.getSocketPath(ctx)

	daemon := &socketDaemon{
		logger:      dela.Logger.With().Str("daemon", socketpath).Logger(),
		socketpath:  socketpath,
		injector:    f.injector,
		actions:     f.actions,
		closing:     make(chan struct{}),
		readTimeout: ioTimeout,
		listenFn:    net.Listen,
	}

	return daemon, nil
}

func (f socketFactory) getSocketPath(ctx cli.Flags) string {
	return filepath.Join(ctx.Path("config"), "daemon.sock")
}
