// Package main implements a ledger based on in-memory components.
//
// Unix example:
//
//  go install
//
//  memcoin --socket /tmp/node1 start --port 2001 &
//  memcoin --socket /tmp/node2 start --port 2002 &
//
//  memcoin --config /tmp/node2 minogrpc join --address 127.0.0.1:2001\
//    $(memcoin --config /tmp/node1 minogrpc token)
//
//  memcoin --config /tmp/node1 ordering setup\
//    --member $(memcoin --config /tmp/node1 ordering export)\
//    --member $(memcoin --config /tmp/node2 ordering export)
//
package main

import (
	"os"

	"go.dedis.ch/dela/cli/node"
	cosipbft "go.dedis.ch/dela/core/ordering/cosipbft/controller"
	mino "go.dedis.ch/dela/mino/minogrpc/controller"
)

func main() {
	builder := node.NewBuilder(
		mino.NewMinimal(),
		cosipbft.NewMinimal(),
	)

	app := builder.Build()

	err := app.Run(os.Args)
	if err != nil {
		panic(err)
	}
}
