// Package main implements a ledger based on in-memory components.
//
// Unix example:
//
//  # Expect GOPATH to be correctly set to have memcoin available.
//  go install
//
//  memcoin --config /tmp/node1 start --listen tcp://127.0.0.1:2001 &
//  memcoin --config /tmp/node2 start --listen tcp://127.0.0.1:2002 &
//  memcoin --config /tmp/node3 start --listen tcp://127.0.0.1:2003 &
//
//  # Share the different certificates among the participants.
//  memcoin --config /tmp/node2 minogrpc join --address //127.0.0.1:2001\
//    $(memcoin --config /tmp/node1 minogrpc token)
//  memcoin --config /tmp/node3 minogrpc join --address //127.0.0.1:2001\
//    $(memcoin --config /tmp/node1 minogrpc token)
//
//  # Create a chain with two members.
//  memcoin --config /tmp/node1 ordering setup\
//    --member $(memcoin --config /tmp/node1 ordering export)\
//    --member $(memcoin --config /tmp/node2 ordering export)
//
//  # Add the third after the chain is set up.
//  memcoin --config /tmp/node1 ordering roster add\
//    --member $(memcoin --config /tmp/node3 ordering export)
//
package main

import (
	"fmt"
	"io"
	"os"

	"go.dedis.ch/dela/cli/node"
	access "go.dedis.ch/dela/contracts/access/controller"
	cosipbft "go.dedis.ch/dela/core/ordering/cosipbft/controller"
	db "go.dedis.ch/dela/core/store/kv/controller"
	pool "go.dedis.ch/dela/core/txn/pool/controller"
	signed "go.dedis.ch/dela/core/txn/signed/controller"
	mino "go.dedis.ch/dela/mino/minogrpc/controller"
	proxy "go.dedis.ch/dela/mino/proxy/http/controller"
)

func main() {
	err := run(os.Args)
	if err != nil {
		fmt.Printf("%+v\n", err)
	}
}

func run(args []string) error {
	return runWithCfg(args, config{Writer: os.Stdout})
}

type config struct {
	Channel chan os.Signal
	Writer  io.Writer
}

func runWithCfg(args []string, cfg config) error {
	builder := node.NewBuilderWithCfg(
		cfg.Channel,
		cfg.Writer,
		db.NewController(),
		mino.NewController(),
		cosipbft.NewController(),
		signed.NewManagerController(),
		pool.NewController(),
		access.NewController(),
		proxy.NewController(),
	)

	app := builder.Build()

	err := app.Run(args)
	if err != nil {
		return err
	}

	return nil
}
