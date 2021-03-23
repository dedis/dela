package evoting

import (
	"encoding/base64"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/execution/native"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/ed25519"
	"go.dedis.ch/dela/dkg"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/kyber/v3/suites"
	"golang.org/x/xerrors"
	"strings"
)

const addrKeySeparator = ":"
const membersSeparator = ","

var suite = suites.MustFind("Ed25519")

// commands defines the commands of the evoting contract. This interface helps in
// testing the contract.
type commands interface {
	genPublicKey(snap store.Snapshot, step execution.Step) error
	encrypt(snap store.Snapshot, step execution.Step) error
	decrypt(snap store.Snapshot, step execution.Step) error
}

const (
	// ContractName is the name of the contract.
	ContractName = "go.dedis.ch/dela.Evoting"

	// MembersArg is the argument's name in the transaction that contains the
	// addresses + public keys of each members that participates in DKG
	// TODO : we should avoid this and find another way to instantiate the collective authority (without user input) cosipbft/controller/mod
	MembersArg = "evoting:members"

	// EncryptValueArg is the argument's name in the transaction that contains the
	// provided value to encrypt.
	EncryptValueArg = "evoting:value"

	// KeyArg is the argument's name in the transaction that contains the
	// key of the ciphertext.
	KeyArg = "evoting:key"

	// CmdArg is the argument's name to indicate the kind of command we want to
	// run on the contract. Should be one of the Command type.
	CmdArg = "evoting:command"

	// credentialAllCommand defines the credential command that is allowed to
	// perform all commands.
	credentialAllCommand = "all"
)


// Command defines a type of command for the value contract
type Command string

const (
	// CmdGetPublicKey defines the command to init the DKG protocol and generate the public key
	CmdGetPublicKey Command = "GETPUBLICKEY"

	// This command helps in testing, voters would send already encrypted ballots.
	// TODO : convert to a Store like command
	// CmdEncrypt defines the command to encrypt a plaintext and store its ciphertext
	CmdEncrypt Command = "ENCRYPT"

	// CmdDecrypt defines the command to decrypt a value and print it
	CmdDecrypt Command = "DECRYPT"
)

// NewCreds creates new credentials for a evoting contract execution. We might
// want to use in the future a separate credential for each command.
func NewCreds(id []byte) access.Credential {
	return access.NewContractCreds(id, ContractName, credentialAllCommand)
}

// RegisterContract registers the value contract to the given execution service.
func RegisterContract(exec *native.Service, c Contract) {
	exec.Set(ContractName, c)
}

// Contract is a smart contract that allows one to execute evoting commands
//
// - implements native.Contract
type Contract struct {
	// index contains all the keys set (and not delete) by this contract so far
	indexEncryptedBallots map[string]struct{}

	// dkgActor is
	dkgActor dkg.Actor

	// onet is
	onet mino.Mino

	//TODO : use some sort of enums
	status int

	// access is the access control service managing this smart contract
	access access.Service

	// accessKey is the access identifier allowed to use this smart contract
	accessKey []byte

	// cmd provides the commands that can be executed by this smart contract
	cmd commands

}

// NewContract creates a new Value contract
// (maybe pass injector to the constructor ?)
func NewContract(aKey []byte, srvc access.Service, dkgActor dkg.Actor, onet mino.Mino) Contract {
	contract := Contract{
		indexEncryptedBallots:     map[string]struct{}{},
		access:    srvc,
		accessKey: aKey,
		dkgActor: dkgActor,
		onet: onet,
		status : 0,
	}

	contract.cmd = evotingCommand{Contract: &contract}
	return contract
}

func (c Contract) Execute(snap store.Snapshot, step execution.Step) error {
	creds := NewCreds(c.accessKey)

	err := c.access.Match(snap, creds, step.Current.GetIdentity())
	if err != nil {
		return xerrors.Errorf("identity not authorized: %v (%v)",
			step.Current.GetIdentity(), err)
	}

	cmd := step.Current.GetArg(CmdArg)
	if len(cmd) == 0 {
		return xerrors.Errorf("'%s' not found in tx arg", CmdArg)
	}

	switch Command(cmd) {
	case CmdGetPublicKey:
		err := c.cmd.genPublicKey(snap, step)
		if err != nil {
			return xerrors.Errorf("failed to GENPUBLICKEY: %v", err)
		}
	case CmdEncrypt:
		err := c.cmd.encrypt(snap, step)
		if err != nil {
			return xerrors.Errorf("failed to ENCRYPT: %v", err)
		}
	case CmdDecrypt:
		err := c.cmd.decrypt(snap, step)
		if err != nil {
			return xerrors.Errorf("failed to DECRYPT: %v", err)
		}

	default:
		return xerrors.Errorf("unknown command: %s", cmd)
	}

	return nil
}


// evotingCommand implements the commands of the evoting contract
//
// - implements commands
type evotingCommand struct {
	*Contract
}

/*
memcoin --config /tmp/node1 pool add\
--key private.key\
--args go.dedis.ch/dela.ContractArg --args go.dedis.ch/dela.Evoting\
--args evoting:members --args $(memcoin --config /tmp/node1 dkg export),$(memcoin --config /tmp/node2 dkg export)\
--args evoting:command --args GETPUBLICKEY
*/

func (e evotingCommand) genPublicKey(snap store.Snapshot, step execution.Step) error {

	if e.status == 0 {
		members := step.Current.GetArg(MembersArg)
		if len(members) == 0 {
			return xerrors.Errorf("'%s' not found in tx arg", MembersArg)
		}

		addrs, pubkeys, err := readMembers(members, e.onet)
		if err != nil {
			return xerrors.Errorf("failed to read roster: %v", err)
		}

		if e.onet.GetAddress() == addrs[0] {
			roster := authority.New(addrs, pubkeys)
			pubkey, err := e.dkgActor.Setup(roster, roster.Len())
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

			err = snap.Set([]byte("publicKey"), pubkeyBuf)
			if err != nil {
				return xerrors.Errorf("failed to set value: %v", err)
			}
		}
		e.status +=1
	}

	return nil
}

func readMembers(input []byte, onet mino.Mino) ([]mino.Address, []crypto.PublicKey, error) {

	members := string(input)

	membersParts := strings.Split(members, membersSeparator)

	addrs := make([]mino.Address, len(membersParts))
	pubkeys := make([]crypto.PublicKey, len(membersParts))

	for i, member := range membersParts {
		addr, pubkey, err := decodeMember(onet, member)
		if err != nil {
			return nil, nil, xerrors.Errorf("failed to decode: %v", err)
		}

		addrs[i] = addr
		pubkeys[i] = pubkey
	}
	return addrs, pubkeys, nil
}

func decodeMember(onet mino.Mino, str string) (mino.Address, crypto.PublicKey, error) {
	parts := strings.Split(str, addrKeySeparator)
	if len(parts) != 2 {
		return nil, nil, xerrors.New("invalid member base64 string")
	}

	// 1. Deserialize the address.
	addrBuf, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, nil, xerrors.Errorf("base64 address: %v", err)
	}

	addr := onet.GetAddressFactory().FromText(addrBuf)

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

func (e evotingCommand) encrypt(snap store.Snapshot, step execution.Step) error {
	return nil
}

func (e evotingCommand) decrypt(snap store.Snapshot, step execution.Step) error {
	return nil
}

