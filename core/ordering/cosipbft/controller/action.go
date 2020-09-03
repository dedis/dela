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
	"go.dedis.ch/dela/core/ordering/cosipbft"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
)

// Members is the structure sent to the setup command so that it can reproduce
// the roster to use.
type Members []struct {
	Addr      []byte
	PublicKey []byte
}

type setupAction struct{}

func (a setupAction) GenerateRequest(flags cli.Flags) ([]byte, error) {
	members := make(Members, 0)

	for _, member := range flags.StringSlice("member") {
		parts := strings.Split(member, ":")

		addr, err := base64.StdEncoding.DecodeString(parts[0])
		if err != nil {
			return nil, err
		}

		pubkey, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, err
		}

		members = append(members, struct {
			Addr      []byte
			PublicKey []byte
		}{
			Addr:      addr,
			PublicKey: pubkey,
		})
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
