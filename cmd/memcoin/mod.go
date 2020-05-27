// Package main implements a ledger based on in-memory components.
package main

import (
	"os"

	"go.dedis.ch/dela/cmd"
	byzcoin "go.dedis.ch/dela/ledger/byzcoin/controller"
	mino "go.dedis.ch/dela/mino/minogrpc/controller"
)

func main() {
	app := cmd.NewApp(mino.NewMinimal(), byzcoin.NewMinimal())

	err := app.Run(os.Args)
	if err != nil {
		panic(err)
	}
}
