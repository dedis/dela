package controller

import (
	"encoding/base64"
	"fmt"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/ed25519"
	"go.dedis.ch/dela/dkg"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/suites"
	"golang.org/x/xerrors"
	"io/ioutil"
	"os"
	"strings"
)

const separator = ":"

var suite = suites.MustFind("Ed25519")

type initAction struct {
}

func (a *initAction) Execute(ctx node.Context) error {
	var dkgPedersen dkg.DKG
	err := ctx.Injector.Resolve(&dkgPedersen)
	if err != nil {
		return xerrors.Errorf("failed to resolve dkg: %v", err)
	}

	actor, err := dkgPedersen.Listen()
	if err != nil {
		return xerrors.Errorf("failed to start the RPC: %v", err)
	}

	ctx.Injector.Inject(actor)
	dela.Logger.Info().Msg("DKG has been initialized successfully")
	return nil
}

type setupAction struct {
}

func (a *setupAction) Execute(ctx node.Context) error {
	roster, err := a.readMembers(ctx)
	if err != nil {
		return xerrors.Errorf("failed to read roster: %v", err)
	}

	var actor dkg.Actor
	err = ctx.Injector.Resolve(&actor)
	if err != nil {
		return xerrors.Errorf("failed to resolve actor: %v", err)
	}

	pubkey, err := actor.Setup(roster, roster.Len())
	if err != nil {
		return xerrors.Errorf("failed to setup DKG: %v", err)
	}

	pubkeyBuf, err := pubkey.MarshalBinary()
	if err != nil {
		return xerrors.Errorf("failed to encode pubkey: %v", err)
	}

	dela.Logger.Info().
		Hex("DKG public key", pubkeyBuf).
		Msg("DKG public key")

	return nil
}

type exportInfoAction struct {
}

func (a *exportInfoAction) Execute(ctx node.Context) error {
	var m mino.Mino
	err := ctx.Injector.Resolve(&m)
	if err != nil {
		return xerrors.Errorf("injector: %v", err)
	}

	addr, err := m.GetAddress().MarshalText()
	if err != nil {
		return xerrors.Errorf("failed to marshal address: %v", err)
	}
	var pubkey kyber.Point
	err = ctx.Injector.Resolve(&pubkey)
	if err != nil {
		return xerrors.Errorf("injector: %v", err)
	}

	pubkeyMarshalled, err := pubkey.MarshalBinary()
	if err != nil {
		return xerrors.Errorf("failed to marshal public key: %v", err)
	}

	desc := base64.StdEncoding.EncodeToString(addr) + separator + base64.StdEncoding.EncodeToString(pubkeyMarshalled)

	fmt.Fprint(ctx.Out, desc)

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
	publicKeyFactory := ed25519.NewPublicKeyFactory()

	pubkeyBuf, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, nil, xerrors.Errorf("base64 public key: %v", err)
	}

	pubkey, err := publicKeyFactory.FromBytes(pubkeyBuf)
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to decode public key: %v", err)
	}

	return addr, pubkey, nil
}

type getPublicKeyAction struct {
}

func (a *getPublicKeyAction) Execute(ctx node.Context) error {
	var actor dkg.Actor
	err := ctx.Injector.Resolve(&actor)
	if err != nil {
		return xerrors.Errorf("failed to resolve actor: %v", err)
	}

	pubkey, err := actor.GetPublicKey()
	if err != nil {
		return xerrors.Errorf("failed to retrieve the public key: %v", err)
	}

	pubkeyBuf, err := pubkey.MarshalBinary()
	if err != nil {
		return xerrors.Errorf("failed to encode pubkey: %v", err)
	}

	dela.Logger.Info().
		Hex("DKG public key", pubkeyBuf).
		Msg("DKG public key")

	return nil
}

type encryptAction struct {
}

func (a *encryptAction) Execute(ctx node.Context) error {
	message := ctx.Flags.String("plaintext")
	KfilePath := ctx.Flags.String("KfilePath")
	CfilePath := ctx.Flags.String("CfilePath")

	var actor dkg.Actor
	err := ctx.Injector.Resolve(&actor)
	if err != nil {
		return xerrors.Errorf("failed to resolve actor: %v", err)
	}

	K, C, _, err := actor.Encrypt([]byte(message))
	if err != nil {
		return xerrors.Errorf("failed to encrypt the plaintext: %v", err)
	}

	Kmarshalled, err := K.MarshalBinary()
	if err != nil {
		return xerrors.Errorf("failed to marshall the K element of the ciphertext pair: %v", err)
	}

	err = ioutil.WriteFile(KfilePath, Kmarshalled, os.ModePerm)
	if err != nil {
		return xerrors.Errorf("failed to write file: %v", err)
	}

	Cmarshalled, err := C.MarshalBinary()
	if err != nil {
		return xerrors.Errorf("failed to marshall the C element of the ciphertext pair: %v", err)
	}

	err = ioutil.WriteFile(CfilePath, Cmarshalled, os.ModePerm)
	if err != nil {
		return xerrors.Errorf("failed to write file: %v", err)
	}

	return nil
}

type decryptAction struct {
}

func (a *decryptAction) Execute(ctx node.Context) error {
	KfilePath := ctx.Flags.String("KfilePath")
	CfilePath := ctx.Flags.String("CfilePath")

	var actor dkg.Actor
	err := ctx.Injector.Resolve(&actor)
	if err != nil {
		return xerrors.Errorf("failed to resolve actor: %v", err)
	}

	Kmarshalled, err := ioutil.ReadFile(KfilePath)
	if err != nil {
		return xerrors.Errorf("failed to read K file: %v", err)
	}

	K := suite.Point()
	err = K.UnmarshalBinary(Kmarshalled)
	if err != nil {
		return xerrors.Errorf("failed to unmarshal K: %v", err)
	}

	Cmarshalled, err := ioutil.ReadFile(CfilePath)
	if err != nil {
		return xerrors.Errorf("failed to read C file: %v", err)
	}

	C := suite.Point()
	err = C.UnmarshalBinary(Cmarshalled)
	if err != nil {
		return xerrors.Errorf("failed to unmarshal C: %v", err)
	}

	message, err := actor.Decrypt(K, C)
	if err != nil {
		return xerrors.Errorf("failed to decrypt (K,C): %v", err)
	}

	dela.Logger.Info().
		Msg(string(message))

	return nil
}