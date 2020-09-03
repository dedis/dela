package controller

import (
	"path/filepath"

	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/core/execution/baremetal"
	"go.dedis.ch/dela/core/execution/baremetal/viewchange"
	"go.dedis.ch/dela/core/ordering/cosipbft"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/core/store/hashtree/binprefix"
	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/core/txn/anon"
	poolimpl "go.dedis.ch/dela/core/txn/pool/gossip"
	"go.dedis.ch/dela/core/validation/simple"
	"go.dedis.ch/dela/cosi/flatcosi"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/gossip"
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
	sub.SetDescription("creates a new chain")
	sub.SetFlags(
		cli.StringSliceFlag{
			Name:     "member",
			Required: true,
			Usage:    "one or several member of the new chain",
		},
	)
	sub.SetAction(builder.MakeAction(setupAction{}))

	sub = cmd.SetSubCommand("export")
	sub.SetDescription("export the node information")
	sub.SetAction(builder.MakeAction(exportAction{}))
}

func (minimal) Inject(flags cli.Flags, inj node.Injector) error {
	var m mino.Mino
	err := inj.Resolve(&m)
	if err != nil {
		return err
	}

	signer := bls.NewSigner()
	cosi := flatcosi.NewFlat(m, signer)
	exec := baremetal.NewExecution()

	rosterFac := authority.NewFactory(m.GetAddressFactory(), cosi.GetPublicKeyFactory())
	exec.Set(viewchange.ContractName, viewchange.NewContract(cosipbft.GetRosterKey(), rosterFac))

	txFac := anon.NewTransactionFactory()
	vs := simple.NewService(exec, txFac)

	pool, err := poolimpl.NewPool(gossip.NewFlat(m, txFac))
	if err != nil {
		return err
	}

	db, err := kv.New(filepath.Join(flags.String("config"), "test.db"))
	if err != nil {
		return err
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
		return err
	}

	inj.Inject(srvc)
	inj.Inject(cosi)

	return nil
}
