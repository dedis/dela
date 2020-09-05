package controller

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/core/execution/baremetal/viewchange"
	"go.dedis.ch/dela/core/ordering"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/core/txn/anon"
	"go.dedis.ch/dela/core/txn/pool"
	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

const separator = ":"

// Service is the expected interface of the ordering service that is extended
// with some additional functions.
type Service interface {
	ordering.Service

	GetRoster() (authority.Authority, error)

	Setup(ctx context.Context, ca crypto.CollectiveAuthority) error
}

// SetupAction is an action to create a new chain with a list of participants.
//
// - implements node.ActionTemplate
type setupAction struct{}

// Execute implements node.ActionTemplate. It reads the list of members and
// requesthe setup to the service.
func (a setupAction) Execute(ctx node.Context) error {
	roster, err := a.readMembers(ctx)
	if err != nil {
		return xerrors.Errorf("failed to read roster: %v", err)
	}

	var srvc Service
	err = ctx.Injector.Resolve(&srvc)
	if err != nil {
		return xerrors.Errorf("injector: %v", err)
	}

	timeout := ctx.Flags.Duration("timeout")

	setupCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err = srvc.Setup(setupCtx, roster)
	if err != nil {
		return xerrors.Errorf("failed to setup: %v", err)
	}

	return nil
}

func (a setupAction) readMembers(ctx node.Context) (authority.Authority, error) {
	members := ctx.Flags.StringSlice("member")

	addrs := make([]mino.Address, len(members))
	pubkeys := make([]crypto.PublicKey, len(members))

	for i, member := range members {
		addr, pubkey, err := decodeMember(ctx, member)
		if err != nil {
			return nil, xerrors.Errorf("failed to decode: %v", err)
		}

		addrs[i] = addr
		pubkeys[i] = pubkey
	}

	return authority.New(addrs, pubkeys), nil
}

// ExportAction is an action to display a base64 string describing the node. It
// can be used to transmit the identity of a node to another one.
//
// - implements node.ActionTemplate
type exportAction struct{}

// Execute implements node.ActionTemplate. It looks for the node address and
// public key and prints "$ADDR_BASE64:$PUBLIC_KEY_BASE64".
func (a exportAction) Execute(ctx node.Context) error {
	var m mino.Mino
	err := ctx.Injector.Resolve(&m)
	if err != nil {
		return xerrors.Errorf("injector: %v", err)
	}

	addr, err := m.GetAddress().MarshalText()
	if err != nil {
		return xerrors.Errorf("failed to marshal address: %v", err)
	}

	var c cosi.CollectiveSigning
	err = ctx.Injector.Resolve(&c)
	if err != nil {
		return xerrors.Errorf("injector: %v", err)
	}

	pubkey, err := c.GetSigner().GetPublicKey().MarshalBinary()
	if err != nil {
		return xerrors.Errorf("failed to marshal public key: %v", err)
	}

	desc := base64.StdEncoding.EncodeToString(addr) + separator +
		base64.StdEncoding.EncodeToString(pubkey)

	fmt.Fprint(ctx.Out, desc)

	return nil
}

// RosterAddAction is an action to require a roster change in the change by
// adding a new member.
//
// - implements node.ActionTemplate
type rosterAddAction struct{}

// Execute implements node.ActionTemplate. It reads the new member and send a
// transaction to require a roster change.
func (rosterAddAction) Execute(ctx node.Context) error {
	var srvc Service
	err := ctx.Injector.Resolve(&srvc)
	if err != nil {
		return xerrors.Errorf("injector: %v", err)
	}

	roster, err := srvc.GetRoster()
	if err != nil {
		return xerrors.Errorf("failed to read roster: %v", err)
	}

	addr, pubkey, err := decodeMember(ctx, ctx.Flags.String("member"))
	if err != nil {
		return xerrors.Errorf("failed to decode member: %v", err)
	}

	cset := authority.NewChangeSet()
	cset.Add(addr, pubkey)

	mgr := viewchange.NewManager(anon.NewManager())

	tx, err := mgr.Make(roster.Apply(cset))
	if err != nil {
		return xerrors.Errorf("transaction manager: %v", err)
	}

	var p pool.Pool
	err = ctx.Injector.Resolve(&p)
	if err != nil {
		return xerrors.Errorf("injector: %v", err)
	}

	err = p.Add(tx)
	if err != nil {
		return xerrors.Errorf("failed to add transaction: %v", err)
	}

	// TODO: listen for the new block and check the tx.

	return nil
}

func decodeMember(ctx node.Context, str string) (mino.Address, crypto.PublicKey, error) {
	parts := strings.Split(str, separator)
	if len(parts) != 2 {
		return nil, nil, xerrors.New("invalid member base64 string")
	}

	// 1. Deserialize the address.
	var m mino.Mino
	err := ctx.Injector.Resolve(&m)
	if err != nil {
		return nil, nil, xerrors.Errorf("injector: %v", err)
	}

	addrBuf, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, nil, xerrors.Errorf("base64 address: %v", err)
	}

	addr := m.GetAddressFactory().FromText(addrBuf)

	// 2. Deserialize the public key.
	var c cosi.CollectiveSigning
	err = ctx.Injector.Resolve(&c)
	if err != nil {
		return nil, nil, xerrors.Errorf("injector: %v", err)
	}

	pubkeyBuf, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, nil, xerrors.Errorf("base64 public key: %v", err)
	}

	pubkey, err := c.GetPublicKeyFactory().FromBytes(pubkeyBuf)
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to decode public key: %v", err)
	}

	return addr, pubkey, nil
}
