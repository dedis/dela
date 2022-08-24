package controller

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/ed25519"
	"go.dedis.ch/dela/dkg"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/suites"
	"golang.org/x/xerrors"
)

// suite is the Kyber suite for Pedersen.
var suite = suites.MustFind("Ed25519")

const separator = ":"
const authconfig = "dkgauthority"

type setupAction struct{}

func (a setupAction) Execute(ctx node.Context) error {
	var actor dkg.Actor

	err := ctx.Injector.Resolve(&actor)
	if err != nil {
		return xerrors.Errorf("failed to resolve actor, did you call listen?: %v", err)
	}

	co, err := getCollectiveAuth(ctx)
	if err != nil {
		return xerrors.Errorf("failed to get collective authority: %v", err)
	}

	t := ctx.Flags.Int("threshold")

	pubkey, err := actor.Setup(co, t)
	if err != nil {
		return xerrors.Errorf("failed to setup: %v", err)
	}

	fmt.Fprintf(ctx.Out, "✅ Setup done.\n🔑 Pubkey: %s", pubkey.String())

	return nil
}

func getCollectiveAuth(ctx node.Context) (crypto.CollectiveAuthority, error) {
	authorities := ctx.Flags.StringSlice("authority")

	addrs := make([]mino.Address, len(authorities))

	pubkeys := make([]crypto.PublicKey, len(authorities))

	for i, auth := range authorities {
		addr, pk, err := decodeAuthority(ctx, auth)
		if err != nil {
			return nil, xerrors.Errorf("failed to decode authority: %v", err)
		}

		addrs[i] = addr
		pubkeys[i] = ed25519.NewPublicKeyFromPoint(pk)
	}

	co := authority.New(addrs, pubkeys)

	return co, nil
}

type listenAction struct {
	pubkey kyber.Point
}

func (a listenAction) Execute(ctx node.Context) error {
	var dkg dkg.DKG

	err := ctx.Injector.Resolve(&dkg)
	if err != nil {
		return xerrors.Errorf("failed to resolve dkg: %v", err)
	}

	actor, err := dkg.Listen()
	if err != nil {
		return xerrors.Errorf("failed to listen: %v", err)
	}

	ctx.Injector.Inject(actor)

	fmt.Fprintf(ctx.Out, "✅  Listen done, actor is created.")

	str, err := encodeAuthority(ctx, a.pubkey)
	if err != nil {
		return xerrors.Errorf("failed to encode authority: %v", err)
	}

	path := filepath.Join(ctx.Flags.Path("config"), authconfig)

	err = os.WriteFile(path, []byte(str), 0755)
	if err != nil {
		return xerrors.Errorf("failed to write authority configuration: %v", err)
	}

	fmt.Fprintf(ctx.Out, "📜 Config file written in %s", path)

	return nil
}

func encodeAuthority(ctx node.Context, pk kyber.Point) (string, error) {
	var m mino.Mino
	err := ctx.Injector.Resolve(&m)
	if err != nil {
		return "", xerrors.Errorf("failed to resolve mino: %v", err)
	}

	addr, err := m.GetAddress().MarshalText()
	if err != nil {
		return "", xerrors.Errorf("failed to marshal address: %v", err)
	}

	pkbuf, err := pk.MarshalBinary()
	if err != nil {
		return "", xerrors.Errorf("failed to marshall pubkey: %v", err)
	}

	id := base64.StdEncoding.EncodeToString(addr) + separator +
		base64.StdEncoding.EncodeToString(pkbuf)

	return id, nil
}

func decodeAuthority(ctx node.Context, str string) (mino.Address, kyber.Point, error) {
	parts := strings.Split(str, separator)
	if len(parts) != 2 {
		return nil, nil, xerrors.New("invalid identity base64 string")
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
	pubkeyBuf, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, nil, xerrors.Errorf("base64 public key: %v", err)
	}

	pubkey := suite.Point()

	err = pubkey.UnmarshalBinary(pubkeyBuf)
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to decode pubkey: %v", err)
	}

	return addr, pubkey, nil
}
