package controller

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/core/execution/baremetal/viewchange"
	"go.dedis.ch/dela/core/ordering/cosipbft"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/core/txn/anon"
	"go.dedis.ch/dela/core/txn/pool"
	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
)

// Member is the structure that contains enough data to reproduce a roster
// member.
type Member struct {
	Addr      []byte
	PublicKey []byte
}

// Members is the structure sent to the setup command so that it can reproduce
// the roster to use.
type Members []Member

type setupAction struct{}

func (a setupAction) GenerateRequest(flags cli.Flags) ([]byte, error) {
	members := make(Members, 0)

	for _, member := range flags.StringSlice("member") {
		var m Member

		err := decodeBase64(member, &m)
		if err != nil {
			return nil, err
		}

		members = append(members, m)
	}

	data, err := json.Marshal(members)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (a setupAction) Execute(ctx node.Context) error {
	roster, err := a.readMembers(ctx)
	if err != nil {
		return err
	}

	var srvc *cosipbft.Service
	err = ctx.Injector.Resolve(&srvc)
	if err != nil {
		return err
	}

	setupCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	err = srvc.Setup(setupCtx, roster)
	if err != nil {
		return err
	}

	return nil
}

func (a setupAction) readMembers(ctx node.Context) (authority.Authority, error) {
	dec := json.NewDecoder(ctx.In)

	var members Members
	err := dec.Decode(&members)
	if err != nil {
		return nil, err
	}

	var c cosi.CollectiveSigning
	err = ctx.Injector.Resolve(&c)
	if err != nil {
		return nil, err
	}

	var m mino.Mino
	err = ctx.Injector.Resolve(&m)
	if err != nil {
		return nil, err
	}

	addrs := make([]mino.Address, len(members))
	pubkeys := make([]crypto.PublicKey, len(members))
	for i, member := range members {
		addr := m.GetAddressFactory().FromText(member.Addr)

		pubkey, err := c.GetPublicKeyFactory().FromBytes(member.PublicKey)
		if err != nil {
			return nil, err
		}

		addrs[i] = addr
		pubkeys[i] = pubkey
	}

	return authority.New(addrs, pubkeys), nil
}

type exportAction struct{}

func (a exportAction) GenerateRequest(cli.Flags) ([]byte, error) {
	return nil, nil
}

func (a exportAction) Execute(ctx node.Context) error {
	var m mino.Mino
	err := ctx.Injector.Resolve(&m)
	if err != nil {
		return err
	}

	addr, err := m.GetAddress().MarshalText()
	if err != nil {
		return err
	}

	var c cosi.CollectiveSigning
	err = ctx.Injector.Resolve(&c)
	if err != nil {
		return err
	}

	pubkey, err := c.GetSigner().GetPublicKey().MarshalBinary()
	if err != nil {
		return err
	}

	desc := base64.StdEncoding.EncodeToString(addr) + ":" + base64.StdEncoding.EncodeToString(pubkey)

	fmt.Fprint(ctx.Out, desc)

	return nil
}

type rosterAddAction struct{}

func (rosterAddAction) GenerateRequest(flags cli.Flags) ([]byte, error) {
	var member Member
	err := decodeBase64(flags.String("member"), &member)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(member)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (rosterAddAction) Execute(ctx node.Context) error {
	var srvc *cosipbft.Service
	err := ctx.Injector.Resolve(&srvc)
	if err != nil {
		return err
	}

	roster, err := srvc.GetRoster()
	if err != nil {
		return err
	}

	dec := json.NewDecoder(ctx.In)

	var member Member
	err = dec.Decode(&member)
	if err != nil {
		return err
	}

	var m mino.Mino
	err = ctx.Injector.Resolve(&m)
	if err != nil {
		return err
	}

	var c cosi.CollectiveSigning
	err = ctx.Injector.Resolve(&c)
	if err != nil {
		return err
	}

	addr := m.GetAddressFactory().FromText(member.Addr)

	pubkey, err := c.GetPublicKeyFactory().FromBytes(member.PublicKey)
	if err != nil {
		return err
	}

	cset := authority.NewChangeSet()
	cset.Add(addr, pubkey)

	tx, err := viewchange.NewTransaction(anon.NewManager(), roster.Apply(cset))
	if err != nil {
		return err
	}

	var p pool.Pool
	err = ctx.Injector.Resolve(&p)
	if err != nil {
		return err
	}

	err = p.Add(tx)
	if err != nil {
		return err
	}

	// TODO: listen for the new block and check the tx.

	return nil
}

func decodeBase64(str string, m *Member) error {
	parts := strings.Split(str, ":")

	addr, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return err
	}

	m.Addr = addr

	pubkey, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return err
	}

	m.PublicKey = pubkey

	return nil
}
