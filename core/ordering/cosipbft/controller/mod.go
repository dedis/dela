package controller

import (
	"encoding"
	"path/filepath"
	"time"

	"go.dedis.ch/dela/crypto"

	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/core/access/darc"
	"go.dedis.ch/dela/core/execution/native"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/ordering/cosipbft"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/core/ordering/cosipbft/blockstore"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/store/hashtree/binprefix"
	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/core/txn/pool"
	poolimpl "go.dedis.ch/dela/core/txn/pool/gossip"
	"go.dedis.ch/dela/core/txn/signed"
	"go.dedis.ch/dela/core/validation/simple"
	"go.dedis.ch/dela/cosi/threshold"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/crypto/loader"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/gossip"
	"go.dedis.ch/dela/serde/json"
	"golang.org/x/xerrors"
)

const privateKeyFile = "private.key"

func blsSigner() encoding.BinaryMarshaler {
	return bls.NewSigner()
}

type minimal struct {
	signerFn func() encoding.BinaryMarshaler
}

// NewMinimal creates a new minimal controller for cosipbft.
func NewMinimal() node.Initializer {
	return minimal{
		signerFn: blsSigner,
	}
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

func (m minimal) OnStart(flags cli.Flags, inj node.Injector) error {
	var onet mino.Mino
	err := inj.Resolve(&onet)
	if err != nil {
		return xerrors.Errorf("injector: %v", err)
	}

	signer, err := m.getSigner(flags)
	if err != nil {
		return xerrors.Errorf("signer: %v", err)
	}

	cosi := threshold.NewThreshold(onet, signer)
	cosi.SetThreshold(threshold.ByzantineThreshold)

	exec := native.NewExecution()
	access := darc.NewService(json.NewContext())

	rosterFac := authority.NewFactory(onet.GetAddressFactory(), cosi.GetPublicKeyFactory())
	cosipbft.RegisterRosterContract(exec, rosterFac, access)

	txFac := signed.NewTransactionFactory()
	vs := simple.NewService(exec, txFac)

	pool, err := poolimpl.NewPool(gossip.NewFlat(onet, txFac))
	if err != nil {
		return xerrors.Errorf("pool: %v", err)
	}

	var db kv.DB
	err = inj.Resolve(&db)
	if err != nil {
		return xerrors.Errorf("injector: %v", err)
	}

	tree := binprefix.NewMerkleTree(db, binprefix.Nonce{})

	param := cosipbft.ServiceParam{
		Mino:       onet,
		Cosi:       cosi,
		Validation: vs,
		Access:     access,
		Pool:       pool,
		DB:         db,
		Tree:       tree,
	}

	err = tree.Load()
	if err != nil {
		return xerrors.Errorf("failed to load tree: %v", err)
	}

	genstore := blockstore.NewGenesisDiskStore(db, types.NewGenesisFactory(rosterFac))

	err = genstore.Load()
	if err != nil {
		return xerrors.Errorf("failed to load genesis: %v", err)
	}

	blockFac := types.NewBlockFactory(vs.GetFactory())
	csFac := authority.NewChangeSetFactory(onet.GetAddressFactory(), cosi.GetPublicKeyFactory())
	linkFac := types.NewLinkFactory(blockFac, cosi.GetSignatureFactory(), csFac)

	blocks := blockstore.NewDiskStore(db, linkFac)

	err = blocks.Load()
	if err != nil {
		return xerrors.Errorf("failed to load blocks: %v", err)
	}

	srvc, err := cosipbft.NewService(param, cosipbft.WithGenesisStore(genstore), cosipbft.WithBlockStore(blocks))
	if err != nil {
		return xerrors.Errorf("service: %v", err)
	}

	inj.Inject(db)
	inj.Inject(srvc)
	inj.Inject(cosi)
	inj.Inject(pool)
	inj.Inject(vs)

	return nil
}

func (minimal) OnStop(inj node.Injector) error {
	var srvc ordering.Service
	err := inj.Resolve(&srvc)
	if err != nil {
		return xerrors.Errorf("injector: %v", err)
	}

	err = srvc.Close()
	if err != nil {
		return xerrors.Errorf("while closing service: %v", err)
	}

	var p pool.Pool
	err = inj.Resolve(&p)
	if err != nil {
		return xerrors.Errorf("injector: %v", err)
	}

	err = p.Close()
	if err != nil {
		return xerrors.Errorf("while closing pool: %v", err)
	}

	return nil
}

func (m minimal) getSigner(flags cli.Flags) (crypto.AggregateSigner, error) {
	loader := loader.NewFileLoader(filepath.Join(flags.Path("config"), privateKeyFile))

	signerdata, err := loader.LoadOrCreate(generator{newFn: m.signerFn})
	if err != nil {
		return nil, xerrors.Errorf("while loading: %v", err)
	}

	signer, err := bls.NewSignerFromBytes(signerdata)
	if err != nil {
		return nil, xerrors.Errorf("while unmarshaling: %v", err)
	}

	return signer, nil
}

type generator struct {
	newFn func() encoding.BinaryMarshaler
}

func (g generator) Generate() ([]byte, error) {
	signer := g.newFn()

	data, err := signer.MarshalBinary()
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal signer: %v", err)
	}

	return data, nil
}
