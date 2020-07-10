package main

import (
	"fmt"
	"os"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/calypso/app/builder"
	"go.dedis.ch/dela/cli"
)

func main() {
	builder := builder.NewBuilder()

	cmdBuilder := builder.SetCommand("write")
	cmdBuilder.SetAction(Store)
	cmdBuilder.SetDescription("Store an encrypted message")
	cmdBuilder.SetFlags(cli.StringFlag{
		Name:     "message",
		Usage:    "The encrypted message",
		Required: true,
	})

	app := builder.Build()

	err := app.Run(os.Args)
	if err != nil {
		dela.Logger.Fatal().Msg(err.Error())
	}
}

// Store saves an encrypted message
func Store(f cli.Flags) error {
	fmt.Println("Hi from the store")
	fmt.Println(f.String("message"))
	return nil
}
