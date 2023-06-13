package controller

import (
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/dkg/pedersen"
	"go.dedis.ch/dela/mino"
	"golang.org/x/xerrors"
)

// NewMinimal returns a new minimal initializer
func NewMinimal() node.Initializer {
	return minimal{
		la: &listenAction{},
	}
}

// minimal is an initializer with the minimum set of commands. Indeed it only
// creates and injects a new DKG
//
// - implements node.Initializer
type minimal struct {
	la *listenAction
}

// Build implements node.Initializer. In this case we don't need any command.
func (m minimal) SetCommands(builder node.Builder) {
	cmd := builder.SetCommand("dkg")
	cmd.SetDescription("DKG service administration")

	sub := cmd.SetSubCommand("listen")
	sub.SetDescription("initialize DKG, create the actor and save the authority configuration")
	sub.SetAction(builder.MakeAction(m.la))

	sub = cmd.SetSubCommand("setup")
	sub.SetDescription("setup the DKG service")
	sub.SetFlags(
		cli.StringSliceFlag{
			Name:  "authority",
			Usage: "<ADDR>:<PK> string, where each token is encoded in base64",
		},
		cli.IntFlag{
			Name:  "threshold",
			Usage: "the threshold of the committee",
		},
	)
	sub.SetAction(builder.MakeAction(setupAction{}))

	sub = cmd.SetSubCommand("encrypt")
	sub.SetDescription("encrypt a message. Outputs <hex(K)>:<hex(C)>:<hex(remainder)>")
	sub.SetFlags(
		cli.StringFlag{
			Name:  "message",
			Usage: "the message to encrypt, encoded in hex",
		},
	)
	sub.SetAction(builder.MakeAction(encryptAction{}))

	sub = cmd.SetSubCommand("decrypt")
	sub.SetDescription("decrypt a message")
	sub.SetFlags(
		cli.StringFlag{
			Name:  "encrypted",
			Usage: "the encrypted string, as <hex(K)>:<hex(C)>",
		},
	)
	sub.SetAction(builder.MakeAction(decryptAction{}))

	sub = cmd.SetSubCommand("verifiableEncrypt")
	sub.SetDescription("encrypt a message and provides a proof. " +
		"Outputs <hex(K)>:<hex(C)>:<hex(Ubar)>:<hex(E)>:<hex(F)>:<hex(remainder)>")
	sub.SetFlags(
		cli.StringFlag{
			Name:  "message",
			Usage: "the message to encrypt, encoded in hex",
		},
		cli.StringFlag{
			Name:  "GBar",
			Usage: "the second generator",
		},
	)
	sub.SetAction(builder.MakeAction(verifiableEncryptAction{}))

	sub = cmd.SetSubCommand("verifiableDecrypt")
	sub.SetDescription("decrypt a message and verify the decryption and encryption proof")
	sub.SetFlags(
		cli.StringFlag{
			Name: "ciphertexts",
			Usage: "a list of ciphertext strings " +
				"<hex(K)>:<hex(C)>:<hex(Ubar)>:<hex(E)>:<hex(F)>[:<hex(k)>:...]",
		},
		cli.StringFlag{
			Name:  "GBar",
			Usage: "the second generator",
		},
	)
	sub.SetAction(builder.MakeAction(verifiableDecryptAction{}))

	sub = cmd.SetSubCommand("reshare")
	sub.SetDescription("reshare the DKG secret")
	sub.SetFlags(
		cli.StringSliceFlag{
			Name:  "authority",
			Usage: "<ADDR>:<PK> string, where each token is encoded in base64",
		},
		cli.IntFlag{
			Name:     "thresholdNew",
			Usage:    "the threshold of the new committee",
			Required: true,
		},
	)
	sub.SetAction(builder.MakeAction(reshareAction{}))
}

// OnStart implements node.Initializer. It creates and registers a pedersen DKG.
func (m minimal) OnStart(ctx cli.Flags, inj node.Injector) error {
	var no mino.Mino
	err := inj.Resolve(&no)
	if err != nil {
		return xerrors.Errorf("failed to resolve mino: %v", err)
	}

	dkg, pubkey := pedersen.NewPedersen(no)

	inj.Inject(dkg)

	pubkeyBuf, err := pubkey.MarshalBinary()
	if err != nil {
		return xerrors.Errorf("failed to encode pubkey: %v", err)
	}

	dela.Logger.Info().
		Hex("public key", pubkeyBuf).
		Msg("perdersen public key")

	// the listen action is expecting the pubkey to be set
	m.la.pubkey = pubkey

	return nil
}

// OnStop implements node.Initializer.
func (minimal) OnStop(node.Injector) error {
	return nil
}
