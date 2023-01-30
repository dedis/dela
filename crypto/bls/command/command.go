// Package command defines cli commands for the bls package.
package command

import (
	"os"

	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/crypto/bls"
)

// Initializer implements the BLS initializer for the crypto CLI.
//
// - implements cli.Initializer
type Initializer struct {
}

// SetCommands implements cli.Initializer.
func (i Initializer) SetCommands(provider cli.Provider) {
	action := action{
		printer: os.Stdout,

		genSigner: bls.NewSigner().MarshalBinary,
		getPubKey: getPubkey,
		readFile:  os.ReadFile,
		saveFile:  saveToFile,
	}

	cmd := provider.SetCommand("bls")
	signer := cmd.SetSubCommand("signer")

	new := signer.SetSubCommand("new")
	new.SetDescription("create a new bls signer")
	new.SetFlags(cli.StringFlag{
		Name:     "save",
		Usage:    "if provided, save the signer to that file",
		Required: false,
	}, cli.BoolFlag{
		Name:     "force",
		Usage:    "in the case it saves the signer, will overwrite if needed",
		Required: false,
	})
	new.SetAction(action.newSignerAction)

	read := signer.SetSubCommand("read")
	read.SetDescription("read a signer")
	read.SetFlags(cli.StringFlag{
		Name:     "path",
		Usage:    "path to the signer's file",
		Required: true,
	}, cli.StringFlag{
		Name:     "format",
		Usage:    "output format: [PUBKEY | BASE64 | BASE64_PUBKEY]",
		Value:    "PUBKEY",
		Required: false,
	})
	read.SetAction(action.loadSignerAction)
}
