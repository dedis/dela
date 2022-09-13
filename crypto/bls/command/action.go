package command

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"

	"go.dedis.ch/dela/crypto"

	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/crypto/bls"
	"golang.org/x/xerrors"
)

// action defines the different cli actions of the BLS commands. Defining
// functions and printer helps in testing the commands.
type action struct {
	printer io.Writer

	genSigner func() ([]byte, error)
	getPubKey func([]byte) (crypto.PublicKey, error)

	readFile func(filename string) ([]byte, error)
	saveFile func(path string, force bool, data []byte) error
}

func (a action) newSignerAction(flags cli.Flags) error {
	data, err := a.genSigner()
	if err != nil {
		return xerrors.Errorf("failed to marshal signer: %v", err)
	}

	switch flags.String("save") {
	case "":
		fmt.Fprintln(a.printer, string(data))
	default:
		err := a.saveFile(flags.String("save"), flags.Bool("force"), data)
		if err != nil {
			return xerrors.Errorf("failed to save files: %v", err)
		}
	}

	return nil
}

func (a action) loadSignerAction(flags cli.Flags) error {
	data, err := a.readFile(flags.Path("path"))
	if err != nil {
		return xerrors.Errorf("failed to read data: %v", err)
	}

	var out []byte

	switch flags.String("format") {
	case "PUBKEY":
		pubkey, err := a.getPubKey(data)
		if err != nil {
			return xerrors.Errorf("failed to get PUBKEY: %v", err)
		}

		out, err = pubkey.MarshalText()
		if err != nil {
			return xerrors.Errorf("failed to marshal pubkey: %v", err)
		}

	case "BASE64_PUBKEY":
		pubkey, err := a.getPubKey(data)
		if err != nil {
			return xerrors.Errorf("failed to get PUBKEY: %v", err)
		}

		buf, err := pubkey.MarshalBinary()
		if err != nil {
			return xerrors.Errorf("failed to marshal pubkey: %v", err)
		}

		out = []byte(base64.StdEncoding.EncodeToString(buf))

	case "BASE64":
		out = []byte(base64.StdEncoding.EncodeToString(data))

	default:
		return xerrors.Errorf("unknown format '%s'", flags.String("format"))
	}

	fmt.Fprintln(a.printer, string(out))

	return nil
}

func saveToFile(path string, force bool, data []byte) error {
	if !force && fileExist(path) {
		return xerrors.Errorf("file '%s' already exist, use --force if you "+
			"want to overwrite", path)
	}

	err := os.WriteFile(path, data, os.ModePerm)
	if err != nil {
		return xerrors.Errorf("failed to write file: %v", err)
	}

	return nil
}

func fileExist(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func getPubkey(data []byte) (crypto.PublicKey, error) {
	signer, err := bls.NewSignerFromBytes(data)
	if err != nil {
		return nil, xerrors.Errorf("failed to unmarshal signer: %v", err)
	}

	return signer.GetPublicKey(), nil
}
