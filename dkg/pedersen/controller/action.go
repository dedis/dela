package controller

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
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
	"log"
	"net/http"
	"os"
	"strings"
)

const separator = ":"
const getPublicKeyEndPoint = "/dkg/pubkey"
const encryptEndPoint = "/dkg/encrypt"
const decryptEndPoint = "/dkg/decrypt"

var suite = suites.MustFind("Ed25519")


// initAction is an action to initialize the DKG protocol
//
// - implements node.ActionTemplate
type initAction struct {
}

// Execute implements node.ActionTemplate. It creates an actor from
// the dkgPedersen instance
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

// setupAction is an action to setup the DKG protocol and generate a collective public key
//
// - implements node.ActionTemplate
type setupAction struct {
}

// Execute implements node.ActionTemplate. It reads the list of members and
// request the setup.
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

/* initHttpServerAction is an action to start an HTTP server handling the following endpoints :
 /dkg/pubkey  : get request that returns the collective public key
 /dkg/encrypt : get request that returns the encryption of "hello"
 /dkg/decrypt : post request that returns the decryption of a ciphertext sent by the client

 - implements node.ActionTemplate
 */
type initHttpServerAction struct {
}

// Wraps the ciphertext pairs
type Ciphertext struct {
	K []byte
	C []byte
}

// Execute implements node.ActionTemplate. It implements the handling of endpoints
// and start the HTTP server
func (a *initHttpServerAction) Execute(ctx node.Context) error {
	portNumber := ctx.Flags.String("portNumber")

	//todo : think of where resolving dkg.Actor ! either now or when handling requests
	var actor dkg.Actor
	err := ctx.Injector.Resolve(&actor)
	if err != nil {
		return xerrors.Errorf("failed to resolve actor: %v", err)
	}

	http.HandleFunc(getPublicKeyEndPoint, func(w http.ResponseWriter, r *http.Request){

		pubkey, err := actor.GetPublicKey()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		pubkeyBuf, err := pubkey.MarshalBinary()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		_, err = fmt.Fprintf(w, string(pubkeyBuf))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	plaintext := "hello"

	K, C, _, err := actor.Encrypt([]byte(plaintext))
	if err != nil {
		return xerrors.Errorf("failed to encrypt the plaintext: %v", err)
	}

	Kmarshalled, err := K.MarshalBinary()
	if err != nil {
		return xerrors.Errorf("failed to marshall the K element of the ciphertext pair: %v", err)
	}

	Cmarshalled, err := C.MarshalBinary()
	if err != nil {
		return xerrors.Errorf("failed to marshall the C element of the ciphertext pair: %v", err)
	}

	http.HandleFunc(encryptEndPoint, func(w http.ResponseWriter, r *http.Request){

		response := Ciphertext{K: Kmarshalled, C: Cmarshalled}
		js, err := json.Marshal(response)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(js)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	http.HandleFunc(decryptEndPoint, func(w http.ResponseWriter, r *http.Request){
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		ciphertext:= new (Ciphertext)
		err = json.NewDecoder(bytes.NewBuffer(body)).Decode(ciphertext)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		K := suite.Point()
		err = K.UnmarshalBinary(ciphertext.K)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		C := suite.Point()
		err = C.UnmarshalBinary(ciphertext.C)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		message, err := actor.Decrypt(K, C)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		_, err = fmt.Fprintf(w, string(message))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

	})

	log.Fatal(http.ListenAndServe(":" + portNumber, nil))

	return nil
}

// getPublicKeyAction is an action that prints the collective public key
//
// - implements node.ActionTemplate
type getPublicKeyAction struct {
}

// Execute implements node.ActionTemplate. It retrieves the collective
// public key from the DKG service and prints it.
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

// encryptAction is an action that encrypt the given plaintext
//
// - implements node.ActionTemplate
type encryptAction struct {
}

// Execute implements node.ActionTemplate. It retrieves the encryption
// of the given plaintext from the DKG service and writes the
// ciphertext pair in files
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

// decryptAction is an action that decrypts the given ciphertext
//
// - implements node.ActionTemplate
type decryptAction struct {
}

// Execute implements node.ActionTemplate. It reads the ciphertext pair,
// it retrieves the decryption from the DKG service and prints the
// corresponding plaintext
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