package cmd

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"sync"
	"syscall"

	"github.com/urfave/cli/v2"
	"go.dedis.ch/fabric"
	"golang.org/x/xerrors"
)

type cliApp struct {
	daemonFactory DaemonFactory
	controllers   []Controller
	injector      Injector
	builder       *cliBuilder
	sigs          chan os.Signal
}

// NewApp returns an new application. It registers the controllers and their
// commands so that they can be executed through the CLI.
//
// - implements cmd.Application
func NewApp(ctrls ...Controller) Application {
	injector := &reflectInjector{
		mapper: make(map[reflect.Type]interface{}),
	}

	actions := &actionMap{}

	factory := socketFactory{
		injector: injector,
		actions:  actions,
	}

	return cliApp{
		daemonFactory: factory,
		controllers:   ctrls,
		injector:      injector,
		builder: &cliBuilder{
			injector:      injector,
			actions:       actions,
			daemonFactory: factory,
		},
		sigs: make(chan os.Signal, 1),
	}
}

// Run implements cmd.Application. It runs the CLI and consequently the command
// provided in the arguments.
func (a cliApp) Run(arguments []string) error {
	for _, controller := range a.controllers {
		controller.Build(a.builder)
	}

	commands := a.builder.build()

	commands = append(commands, &cli.Command{
		Name:  "start",
		Usage: "start the daemon",
		Flags: []cli.Flag{},
		Action: func(c *cli.Context) error {
			daemon, err := a.daemonFactory.DaemonFromContext(c)
			if err != nil {
				return xerrors.Errorf("couldn't make daemon: %v", err)
			}

			err = daemon.Listen()
			if err != nil {
				return xerrors.Errorf("couldn't start the daemon: %v", err)
			}

			defer daemon.Close()

			for _, controller := range a.controllers {
				err = controller.Run(a.injector)
				if err != nil {
					return xerrors.Errorf("couldn't run the controller: %v", err)
				}
			}

			signal.Notify(a.sigs, syscall.SIGINT, syscall.SIGTERM)
			defer signal.Stop(a.sigs)

			<-a.sigs

			fabric.Logger.Trace().Msg("daemon has been stopped")

			return nil
		},
	})

	app := &cli.App{
		Name:  "Dela",
		Usage: "Dedis Ledger Architecture",
		Flags: []cli.Flag{
			&cli.PathFlag{
				Name:  "socket",
				Usage: "path to the daemon socket",
			},
		},
		Commands: commands,
	}

	err := app.Run(arguments)
	if err != nil {
		return xerrors.Errorf("failed to execute the command: %v", err)
	}

	return nil
}

// SocketClient opens a connection to a unix socket daemon to send commands.
//
// - implements cmd.Client
type socketClient struct {
	socketpath string
	out        io.Writer
}

// Send implements cmd.Client. It opens a connection and sends the data to the
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
// manage by the filesystem. A user must have read/write access to send a
// command to the daemon.
//
// - implements cmd.Daemon
type socketDaemon struct {
	sync.WaitGroup
	socketpath string
	injector   Injector
	actions    *actionMap
	closing    chan struct{}
}

// Listen implements cmd.Daemon. It starts the daemon by creating the unix
// socket file to the path.
func (d *socketDaemon) Listen() error {
	dir, _ := filepath.Split(d.socketpath)

	err := os.MkdirAll(dir, 0700)
	if err != nil {
		return xerrors.Errorf("couldn't make path: %v", err)
	}

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
					fabric.Logger.Err(err).Send()
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

	buffer := make([]byte, 2)

	_, err := conn.Read(buffer)
	if err != nil {
		d.sendError(conn, xerrors.Errorf("stream corrupted: %v", err))
		return
	}

	id := binary.LittleEndian.Uint16(buffer)
	action := d.actions.Get(id)

	if action == nil {
		d.sendError(conn, xerrors.Errorf("unknown command '%d'", id))
		return
	}

	actx := Request{
		Injector: d.injector,
		In:       conn,
		Out:      conn,
	}

	err = action.Execute(actx)
	if err != nil {
		d.sendError(conn, xerrors.Errorf("command error: %v", err))
		return
	}
}

func (d *socketDaemon) sendError(conn net.Conn, err error) {
	fmt.Fprintf(conn, "[ERROR] %v", err)
}

// Close implements cmd.Daemon. It closes the daemon and waits for the go
// routines to close.
func (d *socketDaemon) Close() error {
	close(d.closing)
	d.Wait()

	return nil
}

// SocketFactory provides primitives to create a daemon and clients from a CLI
// context.
//
// - implements cmd.DaemonFactory
type socketFactory struct {
	injector Injector
	actions  *actionMap
}

// ClientFromContext implements cmd.DaemonFactory. It creates a client based on
// the flags of the context.
func (f socketFactory) ClientFromContext(ctx Context) (Client, error) {
	client := socketClient{
		socketpath: f.getSocketPath(ctx),
		out:        os.Stdout,
	}

	return client, nil
}

// DaemonFromContext implements cmd.DaemonFactory. It creates a daemon based on
// the flags of the context.
func (f socketFactory) DaemonFromContext(ctx Context) (Daemon, error) {
	daemon := &socketDaemon{
		socketpath: f.getSocketPath(ctx),
		injector:   f.injector,
		actions:    f.actions,
		closing:    make(chan struct{}),
	}

	return daemon, nil
}

func (f socketFactory) getSocketPath(ctx Context) string {
	path := ctx.Path("socket")
	if path == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(os.TempDir(), "dela", "daemon.sock")
		}

		return filepath.Join(homeDir, ".dela", "daemon.sock")
	}

	return path
}

// ActionMap stores actions and assigns a unique index to each.
type actionMap struct {
	list []Action
}

func (m *actionMap) Set(a Action) uint16 {
	m.list = append(m.list, a)
	return uint16(len(m.list) - 1)
}

func (m *actionMap) Get(index uint16) Action {
	if int(index) >= len(m.list) {
		return nil
	}

	return m.list[index]
}
