package controller

import (
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/shuffle"
	"golang.org/x/xerrors"
)

// initAction is an action to initialize the shuffle protocol
//
// - implements node.ActionTemplate
type initAction struct {
}

// Execute implements node.ActionTemplate. It creates an actor from
// the neffShuffle instance
func (a *initAction) Execute(ctx node.Context) error {
	var neffShuffle shuffle.SHUFFLE
	err := ctx.Injector.Resolve(&neffShuffle)
	if err != nil {
		return xerrors.Errorf("failed to resolve shuffle: %v", err)
	}

	actor, _ := neffShuffle.Listen()

	/* if err != nil {
		return xerrors.Errorf("failed to initialize the neff shuffle
	protocol: %v", err)
	} */

	ctx.Injector.Inject(actor)
	dela.Logger.Info().Msg("The shuffle protocol has been initialized successfully")
	return nil
}
