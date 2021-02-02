// This file implements the action of the controller.
//
// Documentation Last Review: 02.02.2021
//

package controller

import (
	"encoding/base64"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/cli/node"
	accessContract "go.dedis.ch/dela/contracts/access"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/execution/native"
	"go.dedis.ch/dela/crypto/bls"
	"golang.org/x/xerrors"
)

// addAction is an action to add one or more identities.
//
// - implements node.ActionTemplate
type addAction struct{}

// Execute implements node.ActionTemplate. It reads the list of identities and
// updates the access.
func (a addAction) Execute(ctx node.Context) error {
	var exec *native.Service
	err := ctx.Injector.Resolve(&exec)
	if err != nil {
		return xerrors.Errorf("failed to resolve native service: %v", err)
	}

	var asrv access.Service
	err = ctx.Injector.Resolve(&asrv)
	if err != nil {
		return xerrors.Errorf("failed to resolve access service: %v", err)
	}

	var accessStore accessStore
	err = ctx.Injector.Resolve(&accessStore)
	if err != nil {
		return xerrors.Errorf("failed to resolve access store: %v", err)
	}

	idsStr := ctx.Flags.StringSlice("identity")
	identities, err := parseIdentities(idsStr)
	if err != nil {
		return xerrors.Errorf("failed to parse identities: %v", err)
	}

	err = asrv.Grant(accessStore, accessContract.NewCreds(aKey[:]), identities...)
	if err != nil {
		return xerrors.Errorf("failed to grant: %v", err)
	}

	dela.Logger.Info().Msgf("access granted to %v", identities)

	return nil
}

func parseIdentities(idsStr []string) ([]access.Identity, error) {
	identities := make([]access.Identity, len(idsStr))

	for i, id := range idsStr {
		idBuf, err := base64.StdEncoding.DecodeString(id)
		if err != nil {
			return nil, xerrors.Errorf("failed to decode pub key '%s': %v", id, err)
		}

		pk, err := bls.NewPublicKey(idBuf)
		if err != nil {
			return nil, xerrors.Errorf("failed to unmarshal identity '%s': %v", id, err)
		}

		identities[i] = pk
	}

	return identities, nil
}
