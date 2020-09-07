package controller

import (
	"path/filepath"
	"time"

	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/core/execution/baremetal"
	"go.dedis.ch/dela/core/ordering/cosipbft"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/core/store/hashtree/binprefix"
	"go.dedis.ch/dela/core/store/kv"
	poolimpl "go.dedis.ch/dela/core/txn/pool/gossip"
	"go.dedis.ch/dela/core/txn/signed"
	"go.dedis.ch/dela/core/validation/simple"
	"go.dedis.ch/dela/cosi/flatcosi"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/gossip"
	"golang.org/x/xerrors"
)

type minimal struct{}

// NewMinimal creates a new minimal controller for cosipbft.
func NewMinimal() node.Initializer {
	return minimal{}
}

func (minimal) SetCommands(builder node.Builder) {
	cmd := builder.SetCommand("ordering")
	cmd.SetDescription("Ordering service administration")

	sub := cmd.SetSubCommand("setup")
	sub.SetDescription("Creates a new chain")
	sub.SetFlags(
		cli.DurationFlag{
			Name:  "timeout",
			Usage: "maximum amount of time to setup",
			Value: 20 * time.Second,
		},
		cli.StringSliceFlag{
			Name:     "member",
			Required: true,
			Usage:    "one or several member of the new chain",
		},
	)
	sub.SetAction(builder.MakeAction(setupAction{}))

	sub = cmd.SetSubCommand("export")
	sub.SetDescription("Export the node information")
	sub.SetAction(builder.MakeAction(exportAction{}))

	sub = cmd.SetSubCommand("roster")
	sub.SetDescription("Roster administration")

	sub = sub.SetSubCommand("add")
	sub.SetDescription("Add a member to the chain")
	sub.SetFlags(
		cli.StringFlag{
			Name:     "member",
			Required: true,
			Usage:    "base64 description of the member to add",
		},
		cli.DurationFlag{
			Name:  "wait",
			Usage: "wait for the transaction to be processed",
		},
	)
	sub.SetAction(builder.MakeAction(rosterAddAction{}))
}

func (minimal) Inject(flags cli.Flags, inj node.Injector) error {
	var m mino.Mino
	err := inj.Resolve(&m)
	if err != nil {
		return xerrors.Errorf("injector: %v", err)
	}

	signer := bls.NewSigner()
	cosi := flatcosi.NewFlat(m, signer)
	exec := baremetal.NewExecution()

	rosterFac := authority.NewFactory(m.GetAddressFactory(), cosi.GetPublicKeyFactory())
	cosipbft.RegisterRosterContract(exec, rosterFac)

	txFac := signed.NewTransactionFactory()
	vs := simple.NewService(exec, txFac)

	pool, err := poolimpl.NewPool(gossip.NewFlat(m, txFac))
	if err != nil {
		return xerrors.Errorf("pool: %v", err)
	}

	db, err := kv.New(filepath.Join(flags.String("config"), "test.db"))
	if err != nil {
		return xerrors.Errorf("db: %v", err)
	}

	param := cosipbft.ServiceParam{
		Mino:       m,
		Cosi:       cosi,
		Validation: vs,
		Pool:       pool,
		DB:         db,
		Tree:       binprefix.NewMerkleTree(db, binprefix.Nonce{}),
	}

	srvc, err := cosipbft.NewService(param)
	if err != nil {
		return xerrors.Errorf("service: %v", err)
	}

	inj.Inject(srvc)
	inj.Inject(cosi)
	inj.Inject(pool)
	inj.Inject(vs)

	return nil
}
