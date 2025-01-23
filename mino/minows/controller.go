package minows

import (
	"fmt"

	ma "github.com/multiformats/go-multiaddr"
	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minows/key"
	"golang.org/x/xerrors"
)

// controller
// - implements node.Initializer
type controller struct{}

// NewController creates a CLI app to start a Minows instance.
func NewController() node.Initializer {
	return controller{}
}

const flagListen = "listen"
const flagPublic = "public"

func (c controller) SetCommands(builder node.Builder) {
	builder.SetStartFlags(
		cli.StringFlag{
			Name: flagListen,
			Usage: "Set the address to listen on (default all interfaces, " +
				"random port)",
			Required: false,
			Value:    "/ip4/0.0.0.0/tcp/0",
		},
		cli.StringFlag{
			Name: flagPublic,
			Usage: "Set the publicly reachable address (" +
				"default listen address)",
			Required: false,
			Value:    "",
		},
	)

	cmd := builder.SetCommand("list")
	sub := cmd.SetSubCommand("address")
	sub.SetDescription("Print this node's full dialable address")
	sub.SetAction(builder.MakeAction(addressAction{}))
}

func (c controller) OnStart(flags cli.Flags, inj node.Injector) error {
	listen, err := ma.NewMultiaddr(flags.String(flagListen))
	if err != nil {
		return xerrors.Errorf("could not parse listen addr: %v", err)
	}

	var db kv.DB
	err = inj.Resolve(&db)
	if err != nil {
		return xerrors.Errorf("could not resolve db: %v", err)
	}

	key, err := minokey.NewKey(db)
	if err != nil {
		return xerrors.Errorf("could not load key: %v", err)
	}

	var public ma.Multiaddr
	p := flags.String(flagPublic)
	if p != "" {
		public, err = ma.NewMultiaddr(p)
		if err != nil {
			return xerrors.Errorf("could not parse public addr: %v", err)
		}
	}

	manager := NewManager()

	m, err := NewMinows(manager, listen, public, key)
	if err != nil {
		return xerrors.Errorf("could not start mino: %v", err)
	}
	inj.Inject(m)
	return nil
}

func (c controller) OnStop(inj node.Injector) error {
	var m *Minows
	err := inj.Resolve(&m)
	if err != nil {
		return xerrors.Errorf("could not resolve mino: %v", err)
	}
	err = m.stop()
	if err != nil {
		return xerrors.Errorf("could not stop mino: %v", err)
	}
	return nil
}

// - implements node.ActionTemplate
type addressAction struct{}

func (a addressAction) Execute(req node.Context) error {
	var m mino.Mino
	err := req.Injector.Resolve(&m)
	if err != nil {
		return xerrors.Errorf("could not resolve: %v", err)
	}
	_, err = fmt.Fprint(req.Out, m.GetAddress())
	if err != nil {
		return err
	}
	return nil
}
