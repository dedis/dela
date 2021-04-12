package controller

import (
	"encoding/base64"
	"fmt"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/ed25519"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/shuffle"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/suites"
	"go.dedis.ch/kyber/v3/util/random"
	"golang.org/x/xerrors"
	"strconv"
	"strings"
)

const separator = ":"

var suite = suites.MustFind("Ed25519")


// initAction is an action to initialize the shuffle protocol
//
// - implements node.ActionTemplate
type initAction struct {
}

// Execute implements node.ActionTemplate. It creates an actor from
// the neffShuffle instance
func (a *initAction) Execute(ctx node.Context) error {
	var neffShuffle shuffle.SHUFFLE
	err := ctx.Injector.Resolve(&neffShuffle)
	if err != nil {
		return xerrors.Errorf("failed to resolve shuffle: %v", err)
	}

	actor, err := neffShuffle.Listen()
	if err != nil {
		return xerrors.Errorf("failed to initialize the neff shuffle protocol: %v", err)
	}

	ctx.Injector.Inject(actor)
	dela.Logger.Info().Msg("The shuffle protocol has been initialized successfully")
	return nil
}

// shuffleAction is an action that shuffles hardcoded ElGamal pairs and verifies their decryption
//
// - implements node.ActionTemplate
type shuffleAction struct {
}

// Execute implements node.ActionTemplate. It reads the list of members,
// request the shuffle and perform decryption.
func (a *shuffleAction) Execute(ctx node.Context) error {
	roster, err := a.readMembers(ctx)
	if err != nil {
		return xerrors.Errorf("failed to read roster: %v", err)
	}

	var actor shuffle.Actor
	err = ctx.Injector.Resolve(&actor)
	if err != nil {
		return xerrors.Errorf("failed to resolve actor: %v", err)
	}

	rand := suite.RandomStream()
	h := suite.Scalar().Pick(rand)
	H := suite.Point().Mul(h, nil)

	k := 5
	X := make([]kyber.Point, k)
	Y := make([]kyber.Point, k)

	for i := 0; i < k; i++ {
		// Embed the message (or as much of it as will fit) into a curve point.
		message := "HELLO" + strconv.Itoa(i)
		M := suite.Point().Embed([]byte(message), random.New())

		// ElGamal-encrypt the point to produce ciphertext (K,C).
		k := suite.Scalar().Pick(random.New())             // ephemeral private key
		K := suite.Point().Mul(k, nil)                      // ephemeral DH public key
		S := suite.Point().Mul(k, H) // ephemeral DH shared secret
		C := S.Add(S, M)                                    // message blinded with secret
		X[i] = K
		Y[i] = C

	}

	KsShuffled, CsShuffled, _, err := actor.Shuffle(roster, "Ed25519", X, Y, H)

	if err != nil {
		return xerrors.Errorf("failed to shuffle: %v", err)
	}

	for i := 0; i < k; i++ {
		K := KsShuffled[i]
		C := CsShuffled[i]

		S := suite.Point().Mul(h, K) // regenerate shared secret
		M := suite.Point().Sub(C, S)      // use to un-blind the message

		message, err := M.Data()           // extract the embedded data
		if err != nil {
			return xerrors.Errorf("failed to decrypt: %v", err)
		}

		dela.Logger.Info().Msg(string(message))
	}

	return nil
}

// exportInfoAction is an action to display a base64 string describing the node.
// It can be used to transmit the identity of a node to another one.
//
// - implements node.ActionTemplate
type exportInfoAction struct {
}

// Execute implements node.ActionTemplate. It looks for the node address and
// public key and prints "$ADDR_BASE64:$PUBLIC_KEY_BASE64".
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

func (a shuffleAction) readMembers(ctx node.Context) (authority.Authority, error) {
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
