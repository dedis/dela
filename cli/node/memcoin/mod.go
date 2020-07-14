// Package main implements a ledger based on in-memory components.
//
//  go run mod.go start
//  go run mod.go --socket ~/Desktop/2001.socket start --port 2001\
//    --clientaddr :8081
//  go run mod.go minogrpc token
//  go run mod.go --socket ~/Desktop/2001.socket minogrpc join\
//    --address 127.0.0.1:2000 --token XX --cert-hash XX
//  go run mod.go calypso setup --pubkeys XX,XX\
//    --addrs 127.0.0.1:2000,127.0.0.1:2001 --threshold 2
//
//
package main

import (
	"os"

	// calypso "go.dedis.ch/dela-apps/calypso/controller"
	"go.dedis.ch/dela/cli/node"
	pedersen "go.dedis.ch/dela/dkg/pedersen/controller"
	byzcoin "go.dedis.ch/dela/ledger/byzcoin/controller"
	httpclient "go.dedis.ch/dela/mino/httpclient/controller"
	mino "go.dedis.ch/dela/mino/minogrpc/controller"
)

func main() {
	builder := node.NewBuilder(
		httpclient.NewMinimal(),
		mino.NewMinimal(),
		byzcoin.NewMinimal(),
		pedersen.NewMinimal(),
		// calypso.NewMinimal(),
	)

	app := builder.Build()

	err := app.Run(os.Args)
	if err != nil {
		panic(err)
	}
}
