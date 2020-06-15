package controller

import (
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/ledger"
	"go.dedis.ch/dela/ledger/byzcoin"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

// Minimal is a minimal initializer for Byzcoin. It allows one to setup a
// ledger.
//
// - implements node.Initializer
type minimal struct{}

// NewMinimal creates a new initializer for Byzcoin with minimal options.
func NewMinimal() node.Initializer {
	return minimal{}
}

// Build implements node.Initializer.
func (m minimal) SetCommands(builder node.Builder) {
	cb := builder.SetCommand("byzcoin")
	cb.SetDescription("Set of commands to administrate the ledger.")

	sub := cb.SetSubCommand("setup")
	sub.SetDescription("setup a new ledger")
	sub.SetAction(builder.MakeAction(setupAction{}))
}

// Run implements node.Initializer.
func (m minimal) Inject(ctx cli.Flags, inj node.Injector) error {
	var no mino.Mino
	err := inj.Resolve(&no)
	if err != nil {
		return xerrors.Errorf("failed to resolve: %v", err)
	}

	ldgr := byzcoin.NewLedger(no, bls.NewSigner())

	actor, err := ldgr.Listen()
	if err != nil {
		return err
	}

	inj.Inject(ldgr)
	inj.Inject(actor)

	return nil
}

// SetupAction is an action to setup a ledger from a list of addresses.
//
// - implements node.ActionTemplate
type setupAction struct{}

func (a setupAction) Do(flags cli.Flags) error {
	return nil
}

// Prepare implements node.ActionTemplate.
func (a setupAction) GenerateRequest(ctx cli.Flags) ([]byte, error) {
	return nil, nil
}

// Execute implements node.ActionTemplate.
func (a setupAction) Execute(req node.Context) error {
	dela.Logger.Info().Msg("Consume an incoming message")

	var actor ledger.Actor

	err := req.Injector.Resolve(actor)
	if err != nil {
		return xerrors.Errorf("failed to resolve: %v", err)
	}

	// TODO: setup from a list of addresses only
	err = actor.Setup(nil)
	if err != nil {
		return err
	}

	return nil
}
