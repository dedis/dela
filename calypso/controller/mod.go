package controller

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"go.dedis.ch/dela/calypso"
	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/crypto/ed25519"
	"go.dedis.ch/dela/dkg/pedersen"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/kyber/v3/suites"
	"golang.org/x/xerrors"
)

// Suite is the Kyber suite for Pedersen.
var suite = suites.MustFind("Ed25519")

// NewMinimal returns a new minimal initializer
func NewMinimal() node.Initializer {
	return minimal{}
}

// minimal is an initializer with the minimum set of commands
//
// - implements node.Initializer
type minimal struct{}

// SetCommands implements node.Initializer
func (m minimal) SetCommands(builder node.Builder) {
	cb := builder.SetCommand("calypso")
	cb.SetDescription("Set of commands to administrate Calypso")

	sub := cb.SetSubCommand("setup")
	sub.SetDescription("setup Calypso and create the distributed key. " +
		"Must be run only once.")
	sub.SetAction(builder.MakeAction(setupAction{}))
	sub.SetFlags(
		cli.StringFlag{
			Name:     "pubkeys",
			Usage:    "a list of public keys in hex strings, separated by commas",
			Required: true,
		},
		cli.StringFlag{
			Name: "addrs",
			Usage: "a list of addresses correponding to the public keys, " +
				"separated by commas",
			Required: true,
		},
		cli.IntFlag{
			Name:     "threshold",
			Usage:    "the minimum number of nodes that is needed to decrypt",
			Required: true,
		},
	)
}

// Inject implements node.Initializer. This function contains the initialization
// code run on each node that wants to support Calypso. We create the dkg actor
// and then use it to create the Calypso, which is then injected as a
// dependency. We will need this dependency in the setup phase.
func (m minimal) Inject(ctx cli.Flags, inj node.Injector) error {
	var no mino.Mino
	err := inj.Resolve(&no)
	if err != nil {
		return xerrors.Errorf("failed to resolve mino: %v", err)
	}

	privkey := suite.Scalar().Pick(suite.RandomStream())
	pubkey := suite.Point().Mul(privkey, nil)

	dkg := pedersen.NewPedersen(privkey, no)

	actor, err := dkg.Listen()
	if err != nil {
		return xerrors.Errorf("failed to listen dkg: %v", err)
	}

	caly := calypso.NewCalypso(actor)

	pubkeyBuf, err := pubkey.MarshalBinary()
	if err != nil {
		return xerrors.Errorf("failed to encode pubkey: %v", err)
	}

	pubkeyHex := hex.EncodeToString(pubkeyBuf)

	fmt.Printf("The Calypso hex pub key is: %s\n", pubkeyHex)

	inj.Inject(caly)

	return nil
}

// setupAction is an action to setup Calypso. This action pefrms the DKG key
// sharing and should only be run once on a node.
//
// - implements node.ActionTemplate
type setupAction struct{}

// GenerateRequest implements node.ActionTemplate
func (a setupAction) GenerateRequest(ctx cli.Flags) ([]byte, error) {
	pubkeysStr := ctx.String("pubkeys")
	if pubkeysStr == "" {
		return nil, xerrors.New("pubkeys not found")
	}

	addrsStr := ctx.String("addrs")
	if addrsStr == "" {
		return nil, xerrors.New("addrs not found")
	}

	threshold := ctx.Int("threshold")

	req := executeRequest{
		Threshold: threshold,
		Pubkeys:   strings.Split(pubkeysStr, ","),
		Addrs:     strings.Split(addrsStr, ","),
	}

	if len(req.Pubkeys) != len(req.Addrs) {
		return nil, xerrors.Errorf("there should be the same number of "+
			"pubkkeys and addrs, but got %d pubkeys and %d addrs: %v",
			len(req.Pubkeys), len(req.Addrs), req)
	}

	buffer, err := json.Marshal(req)
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal the request: %v", err)
	}

	return buffer, nil
}

func (a setupAction) Execute(req node.Context) error {
	var no mino.Mino
	err := req.Injector.Resolve(&no)
	if err != nil {
		return xerrors.Errorf("failed to resolve mino: %v", err)
	}

	var ps calypso.PrivateStorage
	err = req.Injector.Resolve(&ps)
	if err != nil {
		return xerrors.Errorf("failed to resolve calypso: %v", err)
	}

	dec := json.NewDecoder(req.In)

	input := executeRequest{}
	err = dec.Decode(&input)
	if err != nil {
		return xerrors.Errorf("failed to get the request: %v", err)
	}

	pubkeys := make([]ed25519.PublicKey, len(input.Pubkeys))
	addrs := make([]mino.Address, len(input.Addrs))
	for i, keyHex := range input.Pubkeys {
		point := suite.Point()

		keyBuf, err := hex.DecodeString(keyHex)
		if err != nil {
			return xerrors.Errorf("failed to decode hex key: %v", err)
		}

		err = point.UnmarshalBinary(keyBuf)
		if err != nil {
			return xerrors.Errorf("failed to unmarhsal point: %v", err)
		}

		pubkeys[i] = ed25519.NewPublicKeyFromPoint(point)
		addrs[i] = no.GetAddressFactory().FromText([]byte(input.Addrs[i]))
	}

	ca := internalCA{
		players: mino.NewAddresses(addrs...),
		pubkeys: pubkeys,
	}

	pubkey, err := ps.Setup(ca, input.Threshold)
	if err != nil {
		return xerrors.Errorf("failed to setup calypso: %v", err)
	}

	pubkeyBuf, err := pubkey.MarshalBinary()
	if err != nil {
		return xerrors.Errorf("failed to mashal pubkey: %v", err)
	}

	fmt.Printf("Calypso has been successfully setup. "+
		"Here is the Calypso pub key: %s\n", hex.EncodeToString(pubkeyBuf))

	return nil
}

// executeRequest holds the data sent to the deamon
type executeRequest struct {
	Threshold int
	// public keys encoded as hex strings
	Pubkeys []string
	Addrs   []string
}
