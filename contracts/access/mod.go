// Package access implements a native contract to handle access. It allows an
// authorized identity to add access as an {ID, CONTRACT, COMMAND, IDENTITIES}
// quadruplet.
//
// ID is the credential identifier. This identifier is generally defined at the
// contract's creation.
// CONTRACT is the contract name.
// COMMAND specifies the command to grant access to on the contract.
// IDENTITIES is a list of standard base64 encoded bls public keys, separated by
// comas.
//
// Documentation Last Review: 02.02.2021
//
package access

import (
	"encoding/base64"
	"encoding/hex"
	"strings"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/execution/native"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/crypto/bls"
	"golang.org/x/xerrors"
)

const (
	// ContractName is the name of the access contract.
	ContractName = "go.dedis.ch/dela.Access"

	// GrantIDArg is the argument's name in the transaction that contains the
	// provided id to grant
	GrantIDArg = "access:grant_id"

	// GrantContractArg is the argument's name in the transaction that contain
	// the provided contract name to grant the access to.
	GrantContractArg = "access:grant_contract"

	// GrantCommandArg is the argument's name in the transaction that contains
	// the provided command to grant access to.
	GrantCommandArg = "access:grant_command"

	// IdentityArg is the argument's name in the transaction that contains the
	// provided identity to grant access to.
	IdentityArg = "access:identity"

	// CmdArg is the argument's name to indicate the kind of command we want to
	// run on the contract. Should be one of the Command type.
	CmdArg = "access:command"

	// credentialAllCommand defines the credential command that is allowed to
	// perform all commands.
	credentialAllCommand = "all"
)

// Command defines a command for the command contract
type Command string

const (
	// CmdSet defines the command to grant access
	CmdSet Command = "GRANT"
)

// NewCreds creates new credentials for an access contract execution.
func NewCreds(id []byte) access.Credential {
	return access.NewContractCreds(id, ContractName, credentialAllCommand)
}

// RegisterContract registers the access contract to the given execution
// service.
func RegisterContract(exec *native.Service, c Contract) {
	exec.Set(ContractName, c)
}

// Contract is the access contract that allows one to handle access.
//
// - implements native.Contract
type Contract struct {
	// access is the access service that will be modified.
	access access.Service

	// accessKey is the credential's ID allowed to use this smart contract
	accessKey []byte

	store store.Readable
}

// NewContract creates a new access contract
func NewContract(aKey []byte, srvc access.Service, store store.Readable) Contract {
	return Contract{
		access:    srvc,
		accessKey: aKey,
		store:     store,
	}
}

// Execute implements native.Contract
func (c Contract) Execute(snap store.Snapshot, step execution.Step) error {
	creds := NewCreds(c.accessKey)

	err := c.access.Match(c.store, creds, step.Current.GetIdentity())
	if err != nil {
		return xerrors.Errorf("identity not authorized: %v (%v)", step.Current.GetIdentity(), err)
	}

	cmd := step.Current.GetArg(CmdArg)
	if len(cmd) == 0 {
		return xerrors.Errorf("'%s' not found in tx arg", CmdArg)
	}

	switch Command(cmd) {
	case CmdSet:
		err := c.grant(snap, step)
		if err != nil {
			return xerrors.Errorf("failed to SET: %v", err)
		}
	default:
		return xerrors.Errorf("access, unknown command: %s", cmd)
	}

	return nil
}

// grant perform the GRANT command
func (c Contract) grant(snap store.Snapshot, step execution.Step) error {
	idHex := step.Current.GetArg(GrantIDArg)
	if len(idHex) == 0 {
		return xerrors.Errorf("'%s' not found in tx arg", GrantIDArg)
	}

	id, err := hex.DecodeString(string(idHex))
	if err != nil {
		return xerrors.Errorf("failed to decode id from tx arg: %v", err)
	}

	contractName := step.Current.GetArg(GrantContractArg)
	if len(contractName) == 0 {
		return xerrors.Errorf("'%s' not found in tx arg", GrantContractArg)
	}

	commandName := step.Current.GetArg(GrantCommandArg)
	if len(commandName) == 0 {
		return xerrors.Errorf("'%s' not found in tx arg", GrantCommandArg)
	}

	base64IDs := strings.Split(string(step.Current.GetArg(IdentityArg)), ",")
	if len(base64IDs) == 0 || len(base64IDs[0]) == 0 {
		return xerrors.Errorf("'%s' not found in tx arg", IdentityArg)
	}

	identities := make([]access.Identity, len(base64IDs))
	for i, base64ID := range base64IDs {
		identity, err := base64.StdEncoding.DecodeString(string(base64ID))
		if err != nil {
			return xerrors.Errorf("failed to decode base64ID: %v", err)
		}

		pubKey, err := bls.NewPublicKey(identity)
		if err != nil {
			return xerrors.Errorf("failed to get public key: %v", err)
		}

		identities[i] = pubKey
	}

	credential := access.NewContractCreds(id, string(contractName), string(commandName))
	err = c.access.Grant(snap, credential, identities...)
	if err != nil {
		return xerrors.Errorf("failed to grant: %v", err)
	}

	dela.Logger.Info().Str("contract", "access").Msgf("granted %x-%s-%s to %s",
		id, contractName, commandName, identities)

	return nil
}
