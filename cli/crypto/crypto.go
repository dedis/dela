// Package main provides a cli for crypto operations like generating keys or
// displaying specific key formats.
package main

import (
	"fmt"
	"io"
	"os"

	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/ucli"
	bls "go.dedis.ch/dela/crypto/bls/command"
)

var builder cli.Builder = ucli.NewBuilder("crypto", nil)
var printer io.Writer = os.Stderr

func main() {
	err := run(os.Args, bls.Initializer{})
	if err != nil {
		fmt.Fprintf(printer, "%+v\n", err)
	}
}

func run(args []string, inits ...cli.Initializer) error {
	for _, init := range inits {
		init.SetCommands(builder)
	}

	app := builder.Build()
	err := app.Run(args)
	if err != nil {
		return err
	}

	return nil
}
