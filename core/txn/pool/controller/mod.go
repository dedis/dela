// Package controller implements a controller for the pool
//
// Documentation Last Review: 02.02.2021
//
package controller

import (
	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/core/access"
)

const (
	// signerFlag is the flag name containing the path to the private keyfile.
	signerFlag = "key"

	// nonceFlag is the flag name containing the nonce.
	nonceFlag = "nonce"
)

type miniController struct {
}

// NewController creates a new minimal controller for the pool
//
// - implements node.Initializer
func NewController() node.Initializer {
	return miniController{}
}

// SetCommands implements mode.Initializer. It sets the command to interact with
// the pool.
func (miniController) SetCommands(builder node.Builder) {
	cmd := builder.SetCommand("pool")
	cmd.SetDescription("interact with the pool")

	sub := cmd.SetSubCommand("add")
	sub.SetDescription("add a transaction to the pool")
	sub.SetFlags(cli.StringSliceFlag{
		Name:  "args",
		Usage: "list of key-value pairs",
	}, cli.IntFlag{
		Name:     nonceFlag,
		Usage:    "nonce to use",
		Required: false,
		Value:    -1,
	}, cli.StringFlag{
		Name:     signerFlag,
		Usage:    "path to the private keyfile",
		Required: true,
	})
	sub.SetAction(builder.MakeAction(&addAction{
		client: &client{},
	}))
}

// OnStart implements node.Initializer
func (m miniController) OnStart(flags cli.Flags, inj node.Injector) error {
	return nil
}

// OnStop implements node.Initializer
func (miniController) OnStop(inj node.Injector) error {
	return nil
}

// client return monotically increasing nonce
//
// - implements signed.Client
type client struct {
	nonce uint64
}

// GetNonce implements signed.Client
func (c *client) GetNonce(access.Identity) (uint64, error) {
	res := c.nonce
	c.nonce++
	return res, nil
}
