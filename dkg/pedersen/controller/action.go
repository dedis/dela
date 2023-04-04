package controller

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/ed25519"
	"go.dedis.ch/dela/dkg"
	mTypes "go.dedis.ch/dela/dkg/pedersen/types"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/suites"
	"golang.org/x/xerrors"
)

// suite is the Kyber suite for Pedersen.
var suite = suites.MustFind("Ed25519")

const separator = ":"
const authconfig = "dkgauthority"
const resolveActorFailed = "failed to resolve actor, did you call listen?: %v"

type setupAction struct{}

func (a setupAction) Execute(ctx node.Context) error {
	var actor dkg.Actor

	err := ctx.Injector.Resolve(&actor)
	if err != nil {
		return xerrors.Errorf(resolveActorFailed, err)
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
	var d dkg.DKG

	err := ctx.Injector.Resolve(&d)
	if err != nil {
		return xerrors.Errorf("failed to resolve dkg: %v", err)
	}

	actor, err := d.Listen()
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

type encryptAction struct{}

func (a encryptAction) Execute(ctx node.Context) error {
	var actor dkg.Actor

	err := ctx.Injector.Resolve(&actor)
	if err != nil {
		return xerrors.Errorf(resolveActorFailed, err)
	}

	message, err := hex.DecodeString(ctx.Flags.String("message"))
	if err != nil {
		return xerrors.Errorf("failed to decode message: %v", err)
	}

	k, c, remainder, err := actor.Encrypt(message)
	if err != nil {
		return xerrors.Errorf("failed to encrypt: %v", err)
	}

	outStr, err := encodeEncrypted(k, c, remainder)
	if err != nil {
		return xerrors.Errorf("failed to generate output: %v", err)
	}

	fmt.Fprint(ctx.Out, outStr)

	return nil
}

type decryptAction struct{}

func (a decryptAction) Execute(ctx node.Context) error {
	var actor dkg.Actor

	err := ctx.Injector.Resolve(&actor)
	if err != nil {
		return xerrors.Errorf(resolveActorFailed, err)
	}

	encrypted := ctx.Flags.String("encrypted")

	k, c, err := decodeEncrypted(encrypted)
	if err != nil {
		return xerrors.Errorf("failed to decode encrypted str: %v", err)
	}

	decrypted, err := actor.Decrypt(k, c)
	if err != nil {
		return xerrors.Errorf("failed to decrypt: %v", err)
	}

	fmt.Fprint(ctx.Out, hex.EncodeToString(decrypted))

	return nil
}

type reencryptAction struct{}

func (a reencryptAction) Execute(ctx node.Context) error {
	var actor dkg.Actor

	err := ctx.Injector.Resolve(&actor)
	if err != nil {
		return xerrors.Errorf(resolveActorFailed, err)
	}

	// first, let's decrypt the given data
	encrypted := ctx.Flags.String("encrypted")

	k, c, err := decodeEncrypted(encrypted)
	if err != nil {
		return xerrors.Errorf("failed to decode encrypted str: %v", err)
	}

	decrypted, err := actor.Decrypt(k, c)
	if err != nil {
		return xerrors.Errorf("failed to decrypt: %v", err)
	}

	fmt.Fprint(ctx.Out, hex.EncodeToString(decrypted))

	// second, let's encrypt 'decrypted' with the given public key
	publickey := ctx.Flags.String("publickey")

	pk, err := decodePublickey(publickey)
	if err != nil {
		return xerrors.Errorf("failed to decode public key str: %v", err)
	}

	k, c, remainder, err := actor.EncryptWithPublicKey(decrypted, pk)
	if err != nil {
		return xerrors.Errorf("failed to reencrypt: %v", err)
	}

	outStr, err := encodeEncrypted(k, c, remainder)
	if err != nil {
		return xerrors.Errorf("failed to generate output: %v", err)
	}

	fmt.Fprint(ctx.Out, outStr)

	return nil
}

func decodePublickey(str string) (pk kyber.Point, err error) {
	// Decode public key
	publickey, err := hex.DecodeString(str)
	if err != nil {
		return nil, xerrors.Errorf("failed to decode pk: %v", err)
	}

	pk = suite.Point()

	err = pk.UnmarshalBinary(publickey)
	if err != nil {
		return nil, xerrors.Errorf("failed to unmarshal pk point: %v", err)
	}

	return pk, nil
}

func encodeEncrypted(k, c kyber.Point, remainder []byte) (string, error) {
	kbuff, err := k.MarshalBinary()
	if err != nil {
		return "", xerrors.Errorf("failed to marshal k: %v", err)
	}

	cbuff, err := c.MarshalBinary()
	if err != nil {
		return "", xerrors.Errorf("failed to marshal c: %v", err)
	}

	encoded := hex.EncodeToString(kbuff) + separator +
		hex.EncodeToString(cbuff) + separator +
		hex.EncodeToString(remainder)

	return encoded, nil
}

func decodeEncrypted(str string) (k kyber.Point, c kyber.Point, err error) {
	parts := strings.Split(str, separator)
	if len(parts) < 2 {
		return nil, nil, xerrors.Errorf("malformed encoded: %s", str)
	}

	// Decode K
	kbuff, err := hex.DecodeString(parts[0])
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to decode k point: %v", err)
	}

	k = suite.Point()

	err = k.UnmarshalBinary(kbuff)
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to unmarshal k point: %v", err)
	}

	// Decode C
	cbuff, err := hex.DecodeString(parts[1])
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to decode c point: %v", err)
	}

	c = suite.Point()

	err = c.UnmarshalBinary(cbuff)
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to unmarshal c point: %v", err)
	}

	return k, c, nil
}

// Verifiable encryption

type verifiableEncryptAction struct{}

func (a verifiableEncryptAction) Execute(ctx node.Context) error {
	var actor dkg.Actor

	err := ctx.Injector.Resolve(&actor)
	if err != nil {
		return xerrors.Errorf(resolveActorFailed, err)
	}

	// Decode GBar
	gBarbuff, err := hex.DecodeString(ctx.Flags.String("GBar"))
	if err != nil {
		return xerrors.Errorf("failed to decode GBar point: %v", err)
	}

	gBar := suite.Point()

	err = gBar.UnmarshalBinary(gBarbuff)
	if err != nil {
		return xerrors.Errorf("failed to unmarshal GBar point: %v", err)
	}

	// Decode the message
	message, err := hex.DecodeString(ctx.Flags.String("message"))
	if err != nil {
		return xerrors.Errorf("failed to decode message: %v", err)
	}

	ciphertext, remainder, err := actor.VerifiableEncrypt(message, gBar)
	if err != nil {
		return xerrors.Errorf("failed to encrypt: %v", err)
	}

	// Encoding the ciphertext
	// Encoding K
	kbuff, err := ciphertext.K.MarshalBinary()
	if err != nil {
		return xerrors.Errorf("failed to marshal k: %v", err)
	}

	// Encoding C
	cbuff, err := ciphertext.C.MarshalBinary()
	if err != nil {
		return xerrors.Errorf("failed to marshal c: %v", err)
	}

	// Encoding Ubar
	uBarbuff, err := ciphertext.UBar.MarshalBinary()
	if err != nil {
		return xerrors.Errorf("failed to marshal Ubar: %v", err)
	}

	// Encoding E
	ebuff, err := ciphertext.E.MarshalBinary()
	if err != nil {
		return xerrors.Errorf("failed to marshal E: %v", err)
	}

	// Encoding F
	fbuff, err := ciphertext.F.MarshalBinary()
	if err != nil {
		return xerrors.Errorf("failed to marshal F: %v", err)
	}

	outStr := hex.EncodeToString(kbuff) + separator +
		hex.EncodeToString(cbuff) + separator +
		hex.EncodeToString(uBarbuff) + separator +
		hex.EncodeToString(ebuff) + separator +
		hex.EncodeToString(fbuff) + separator +
		hex.EncodeToString(remainder)

	fmt.Fprint(ctx.Out, outStr)

	return nil
}

// Verifiable decrypt

type verifiableDecryptAction struct{}

func (a verifiableDecryptAction) Execute(ctx node.Context) error {
	var actor dkg.Actor

	err := ctx.Injector.Resolve(&actor)
	if err != nil {
		return xerrors.Errorf(resolveActorFailed, err)
	}

	// Decode GBar
	gBarbuff, err := hex.DecodeString(ctx.Flags.String("GBar"))
	if err != nil {
		return xerrors.Errorf("failed to decode GBar point: %v", err)
	}

	gBar := suite.Point()

	err = gBar.UnmarshalBinary(gBarbuff)
	if err != nil {
		return xerrors.Errorf("failed to unmarshal GBar point: %v", err)
	}

	// Decode the ciphertexts
	var ciphertextSlice []mTypes.Ciphertext

	ciphertextString := ctx.Flags.String("ciphertexts")

	parts := strings.Split(ciphertextString, separator)
	if len(parts)%5 != 0 {
		return xerrors.Errorf("malformed encoded: %s", ciphertextString)
	}

	batchSize := len(parts) / 5

	for i := 0; i < batchSize; i++ {

		// Decode K
		kbuff, err := hex.DecodeString(parts[i*5])
		if err != nil {
			return xerrors.Errorf("failed to decode k point: %v", err)
		}

		k := suite.Point()

		err = k.UnmarshalBinary(kbuff)
		if err != nil {
			return xerrors.Errorf("failed to unmarshal k point: %v", err)
		}

		// Decode C
		cbuff, err := hex.DecodeString(parts[i*5+1])
		if err != nil {
			return xerrors.Errorf("failed to decode c point: %v", err)
		}

		c := suite.Point()

		err = c.UnmarshalBinary(cbuff)
		if err != nil {
			return xerrors.Errorf("failed to unmarshal c point: %v", err)
		}

		// Decode UBar
		uBarbuff, err := hex.DecodeString(parts[i*5+2])
		if err != nil {
			return xerrors.Errorf("failed to decode UBar point: %v", err)
		}

		uBar := suite.Point()

		err = uBar.UnmarshalBinary(uBarbuff)
		if err != nil {
			return xerrors.Errorf("failed to unmarshal UBar point: %v", err)
		}

		// Decode E
		ebuff, err := hex.DecodeString(parts[i*5+3])
		if err != nil {
			return xerrors.Errorf("failed to decode E: %v", err)
		}

		e := suite.Scalar()

		err = e.UnmarshalBinary(ebuff)
		if err != nil {
			return xerrors.Errorf("failed to unmarshal E: %v", err)
		}

		// Decode F
		fbuff, err := hex.DecodeString(parts[i*5+4])
		if err != nil {
			return xerrors.Errorf("failed to decode F: %v", err)
		}

		f := suite.Scalar()

		err = f.UnmarshalBinary(fbuff)
		if err != nil {
			return xerrors.Errorf("failed to unmarshal F: %v", err)
		}

		ciphertextStruct := mTypes.Ciphertext{
			K:    k,
			C:    c,
			UBar: uBar,
			E:    e,
			F:    f,
			GBar: gBar,
		}

		ciphertextSlice = append(ciphertextSlice, ciphertextStruct)

	}

	decrypted, err := actor.VerifiableDecrypt(ciphertextSlice)
	if err != nil {
		return xerrors.Errorf("failed to decrypt: %v", err)
	}

	var decryptString []string

	for i := 0; i < batchSize; i++ {
		decryptString = append(decryptString, hex.EncodeToString(decrypted[i]))
	}

	fmt.Fprint(ctx.Out, decryptString)
	return nil
}

// reshare

type reshareAction struct{}

func (a reshareAction) Execute(ctx node.Context) error {
	var actor dkg.Actor

	err := ctx.Injector.Resolve(&actor)
	if err != nil {
		return xerrors.Errorf(resolveActorFailed, err)
	}

	co, err := getCollectiveAuth(ctx)
	if err != nil {
		return xerrors.Errorf("failed to get collective authority: %v", err)
	}

	t := ctx.Flags.Int("thresholdNew")

	err = actor.Reshare(co, t)
	if err != nil {
		return xerrors.Errorf("failed to reshare: %v", err)
	}

	fmt.Fprintf(ctx.Out, "✅ Reshare done.\n")

	return nil
}
