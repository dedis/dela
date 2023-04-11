// Package calypso implements a Calypso contract that can advertise Secret
// Management Committees and deal with secrets.
package calypso

import (
	"fmt"
	"io"
	"net"
	"sort"
	"strings"

	"go.dedis.ch/dela"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/execution/native"
	"go.dedis.ch/dela/core/store"
	"golang.org/x/xerrors"
)

// commands defines the commands of the calypso contract.
// This interface helps in testing the contract.
type commands interface {
	advertiseSmc(snap store.Snapshot, step execution.Step) error
	deleteSmc(snap store.Snapshot, step execution.Step) error
	listSmc(snap store.Snapshot) error

	createSecret(snap store.Snapshot, step execution.Step) error
	listSecrets(snap store.Snapshot, step execution.Step) error
}

const (
	// ContractName is the name of the contract.
	ContractName = "go.dedis.ch/calypso.SMC"

	// KeyArg is the argument's name in the transaction that contains the
	// public key of the SMC to update.
	KeyArg = "calypso:smc_key"

	// RosterArg is the argument's name in the transaction that contains the
	// roster to associate with a given public key.
	RosterArg = "calypso:smc_roster"

	// SecretNameArg is the argument's name in the transaction that contains
	// the name of the secret to be published on the blockchain.
	SecretNameArg = "calypso:secret_name"

	// SecretArg is the argument's name in the transaction that contains the
	// secret to be published on the blockchain.
	SecretArg = "calypso:secret_value"

	// CmdArg is the argument's name to indicate the kind of command we want to
	// run on the contract. Should be one of the Command type.
	CmdArg = "calypso:command"

	// credentialAllCommand defines the credential command that is allowed to
	// perform all commands.
	credentialAllCommand = "all"
)

// Command defines a type of command for the value contract
type Command string

const (
	// CmdAdvertiseSmc defines the command to advertise a SMC
	CmdAdvertiseSmc Command = "ADVERTISE_SMC"

	// CmdDeleteSmc defines a command to deleteSmc a SMC
	CmdDeleteSmc Command = "DELETE_SMC"

	// CmdListSmc defines a command to list all SMCs (not deleted) so far.
	CmdListSmc Command = "LIST_SMC"

	// CmdCreateSecret defines a command to create a new secret.
	CmdCreateSecret Command = "CREATE_SECRET"

	// CmdListSecrets defines a command to list secrets for a SMC.
	CmdListSecrets Command = "LIST_SECRETS"
)

// Common error messages
const (
	notFoundInTxArg = "'%s' not found in tx arg"
)

// NewCreds creates new credentials for a value contract execution. We might
// want to use in the future a separate credential for each command.
func NewCreds(id []byte) access.Credential {
	return access.NewContractCreds(id, ContractName, credentialAllCommand)
}

// RegisterContract registers the value contract to the given execution service.
func RegisterContract(exec *native.Service, c Contract) {
	exec.Set(ContractName, c)
}

// Contract is a simple smart contract that allows one to handle the storage by
// performing CRUD operations.
//
// - implements native.Contract
type Contract struct {
	// index contains all the keys set (and not deleteSmc) by this contract so far
	index map[string]struct{}

	// secrets contains a mapping between the keys and their associated secrets
	secrets map[string][]string

	// access is the access control service managing this smart contract
	access access.Service

	// accessKey is the access identifier allowed to use this smart contract
	accessKey []byte

	// cmd provides the commands that can be executed by this smart contract
	cmd commands

	// printer is the output used by the READ and LIST commands
	printer io.Writer
}

// NewContract creates a new Value contract
func NewContract(aKey []byte, srvc access.Service) Contract {
	contract := Contract{
		index:     map[string]struct{}{},
		secrets:   map[string][]string{},
		access:    srvc,
		accessKey: aKey,
		printer:   infoLog{},
	}

	contract.cmd = calypsoCommand{Contract: &contract}

	return contract
}

// Execute implements native.Contract. It runs the appropriate command.
func (c Contract) Execute(snap store.Snapshot, step execution.Step) error {
	creds := NewCreds(c.accessKey)

	err := c.access.Match(snap, creds, step.Current.GetIdentity())
	if err != nil {
		return xerrors.Errorf("identity not authorized: %v (%v)",
			step.Current.GetIdentity(), err)
	}

	cmd := step.Current.GetArg(CmdArg)
	if len(cmd) == 0 {
		return xerrors.Errorf(notFoundInTxArg, CmdArg)
	}

	switch Command(cmd) {
	case CmdAdvertiseSmc:
		err := c.cmd.advertiseSmc(snap, step)
		if err != nil {
			return xerrors.Errorf("failed to ADVERTISE_SMC: %v", err)
		}
	case CmdDeleteSmc:
		err := c.cmd.deleteSmc(snap, step)
		if err != nil {
			return xerrors.Errorf("failed to DELETE_SMC: %v", err)
		}
	case CmdListSmc:
		err := c.cmd.listSmc(snap)
		if err != nil {
			return xerrors.Errorf("failed to LIST_SMC: %v", err)
		}
	case CmdCreateSecret:
		err := c.cmd.createSecret(snap, step)
		if err != nil {
			return xerrors.Errorf("failed to CREATE_SECRET: %v", err)
		}
	case CmdListSecrets:
		err := c.cmd.listSecrets(snap, step)
		if err != nil {
			return xerrors.Errorf("failed to LIST_SECRETS: %v", err)
		}
	default:
		return xerrors.Errorf("unknown command: %s", cmd)
	}

	return nil
}

// calypsoCommand implements the commands of the value contract
//
// - implements commands
type calypsoCommand struct {
	*Contract
}

// advertiseSmc implements commands. It performs the ADVERTISE_SMC command
func (c calypsoCommand) advertiseSmc(snap store.Snapshot, step execution.Step) error {
	key := step.Current.GetArg(KeyArg)
	if len(key) == 0 {
		return xerrors.Errorf(notFoundInTxArg, KeyArg)
	}

	roster := step.Current.GetArg(RosterArg)
	if len(roster) == 0 {
		return xerrors.Errorf(notFoundInTxArg, RosterArg)
	}

	nodeList := strings.Split(string(roster), ",")
	for _, r := range nodeList {
		_, _, err := net.SplitHostPort(r)
		if err != nil {
			return xerrors.Errorf("invalid node '%s' in roster: %v", r, err)
		}
	}

	err := snap.Set(key, roster)
	if err != nil {
		return xerrors.Errorf("failed to set roster: %v", err)
	}

	c.index[string(key)] = struct{}{}
	c.secrets[string(key)] = []string{}

	dela.Logger.Info().Str("contract", ContractName).Msgf("setting %x=%s", key, roster)

	return nil
}

// deleteSmc implements commands. It performs the DELETE_SMC command
func (c calypsoCommand) deleteSmc(snap store.Snapshot, step execution.Step) error {
	key := step.Current.GetArg(KeyArg)
	if len(key) == 0 {
		return xerrors.Errorf(notFoundInTxArg, KeyArg)
	}

	err := snap.Delete(key)
	if err != nil {
		return xerrors.Errorf("failed to deleteSmc key '%x': %v", key, err)
	}

	for _, secret := range c.secrets[string(key)] {
		dela.Logger.Info().
			Msgf("Deleting secret '%s' that depended on deleted SMC '%s'", secret, key)

		err = snap.Delete([]byte(secret))
		if err != nil {
			dela.Logger.Warn().
				Msgf("Could not delete secret '%s', "+
					"orphaned by deleted SMC '%s'", secret, key)
		}
	}

	delete(c.index, string(key))
	delete(c.secrets, string(key))

	return nil
}

// listSmc implements commands. It performs the LIST_SMC command
func (c calypsoCommand) listSmc(snap store.Snapshot) error {
	res := []string{}

	for k := range c.index {
		v, err := snap.Get([]byte(k))
		if err != nil {
			return xerrors.Errorf("failed to get key '%s': %v", k, err)
		}

		res = append(res, fmt.Sprintf("%x=%s", k, v))
	}

	sort.Strings(res)
	fmt.Fprint(c.printer, strings.Join(res, ","))

	return nil
}

// createSecret implements commands. It performs the CREATE_SECRET command
func (c calypsoCommand) createSecret(snap store.Snapshot, step execution.Step) error {

	key := step.Current.GetArg(KeyArg)
	if len(key) == 0 {
		return xerrors.Errorf(notFoundInTxArg, KeyArg)
	}

	name := step.Current.GetArg(SecretNameArg)
	if len(name) == 0 {
		return xerrors.Errorf(notFoundInTxArg, SecretNameArg)
	}

	secret := step.Current.GetArg(SecretArg)
	if len(secret) == 0 {
		return xerrors.Errorf(notFoundInTxArg, SecretArg)
	}

	_, ok := c.index[string(key)]
	if !ok {
		return xerrors.Errorf("'%s' was not found among the SMCs", key)
	}

	err := snap.Set(name, secret)
	if err != nil {
		return xerrors.Errorf("failed to set secret: %v", err)
	}

	c.secrets[string(key)] = append(c.secrets[string(key)], string(name))

	dela.Logger.Info().
		Str("contract", ContractName).
		Msgf("setting secret %x=%s", name, secret)

	return nil
}

// listSecrets implements commands. It performs the LIST_SECRETS command
func (c calypsoCommand) listSecrets(snap store.Snapshot, step execution.Step) error {
	res := []string{}

	key := step.Current.GetArg(KeyArg)
	if len(key) == 0 {
		return xerrors.Errorf(notFoundInTxArg, KeyArg)
	}

	_, found := c.secrets[string(key)]
	if !found {
		return xerrors.Errorf("SMC not found: %s", key)
	}

	for _, k := range c.secrets[string(key)] {
		v, err := snap.Get([]byte(k))
		if err != nil {
			return xerrors.Errorf("failed to get key '%s': %v", k, err)
		}

		res = append(res, fmt.Sprintf("%s=%s", k, v))
	}

	sort.Strings(res)
	fmt.Fprint(c.printer, strings.Join(res, ","))

	return nil
}

// infoLog defines an output using zerolog
//
// - implements io.writer
type infoLog struct{}

func (h infoLog) Write(p []byte) (int, error) {
	dela.Logger.Info().Msg(string(p))

	return len(p), nil
}
