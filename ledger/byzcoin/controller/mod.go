package controller

import (
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/cmd"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/ledger"
	"go.dedis.ch/dela/ledger/byzcoin"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

type minimal struct{}

// NewMinimal creates a new controller for Byzcoin with minimal options.
func NewMinimal() cmd.Controller {
	return minimal{}
}

func (m minimal) Build(builder cmd.Builder) {
	cli := builder.Command("byzcoin").
		Description("Set of commands to administrate the ledger.")

	cli.Command("setup").
		Description("setup a new ledger").
		Action(setupAction{})
}

func (m minimal) Run(inj cmd.Injector) error {
	var no mino.Mino
	err := inj.Resolve(&no)
	if err != nil {
		return xerrors.Errorf("failed to inject: %v", err)
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

type setupAction struct{}

func (a setupAction) Prepare(ctx cmd.Context) ([]byte, error) {
	return nil, nil
}

func (a setupAction) Execute(req cmd.Request) error {
	dela.Logger.Info().Msg("Consume an incoming message")

	var actor ledger.Actor

	err := req.Injector.Resolve(actor)
	if err != nil {
		return xerrors.Errorf("failed to inject: %v", err)
	}

	// TODO: setup from a list of addresses only
	err = actor.Setup(nil)
	if err != nil {
		return err
	}

	return nil
}
