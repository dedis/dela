// Package calypso implements a Calypso contract that can advertise Secret
// Management Committees and deal with secrets.
package calypso

import (
	"fmt"
	"io"
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
}

const (
	// ContractName is the name of the contract.
	ContractName = "go.dedis.ch/calypso.SMC"

	// KeyArg is the argument's name in the transaction that contains the
	// public key of the SMC to update.
	KeyArg = "calypso:public_key"

	// RosterArg is the argument's name in the transaction that contains the
	// roster to associate with a given public key.
	RosterArg = "calypso:roster"

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

	// CmdListSmc defines a command to listSmc all SMCs (and not deleted) so far.
	CmdListSmc Command = "LIST_SMC"
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
		return xerrors.Errorf("'%s' not found in tx arg", CmdArg)
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

// advertiseSmc implements commands. It performs the WRITE command
func (c calypsoCommand) advertiseSmc(snap store.Snapshot, step execution.Step) error {
	key := step.Current.GetArg(KeyArg)
	if len(key) == 0 {
		return xerrors.Errorf("'%s' not found in tx arg", KeyArg)
	}

	value := step.Current.GetArg(RosterArg)
	if len(value) == 0 {
		return xerrors.Errorf("'%s' not found in tx arg", RosterArg)
	}

	err := snap.Set(key, value)
	if err != nil {
		return xerrors.Errorf("failed to set value: %v", err)
	}

	c.index[string(key)] = struct{}{}

	dela.Logger.Info().Str("contract", ContractName).Msgf("setting %x=%s", key, value)

	return nil
}

// deleteSmc implements commands. It performs the DELETE command
func (c calypsoCommand) deleteSmc(snap store.Snapshot, step execution.Step) error {
	key := step.Current.GetArg(KeyArg)
	if len(key) == 0 {
		return xerrors.Errorf("'%s' not found in tx arg", KeyArg)
	}

	err := snap.Delete(key)
	if err != nil {
		return xerrors.Errorf("failed to deleteSmc key '%x': %v", key, err)
	}

	delete(c.index, string(key))

	return nil
}

// listSmc implements commands. It performs the LIST command
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

// infoLog defines an output using zerolog
//
// - implements io.writer
type infoLog struct{}

func (h infoLog) Write(p []byte) (int, error) {
	dela.Logger.Info().Msg(string(p))

	return len(p), nil
}
