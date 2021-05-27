package evoting

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/contracts/evoting/types"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/execution/native"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/proof"
	shuffleKyber "go.dedis.ch/kyber/v3/shuffle"

	// shuffleKyber "go.dedis.ch/kyber/v3/shuffle"
	"go.dedis.ch/kyber/v3/suites"
	"golang.org/x/xerrors"
)

const protocolName = "PairShuffle"
const messageOnlyOneShufflePerRound = "Only one shuffle per round is allowed"

var suite = suites.MustFind("Ed25519")

// commands defines the commands of the evoting contract.
type commands interface {
	createElection(snap store.Snapshot, step execution.Step) error
	castVote(snap store.Snapshot, step execution.Step) error
	closeElection(snap store.Snapshot, step execution.Step) error
	shuffleBallots(snap store.Snapshot, step execution.Step) error
	decryptBallots(snap store.Snapshot, step execution.Step) error
	cancelElection(snap store.Snapshot, step execution.Step) error
}

const (
	// ContractName is the name of the contract.
	ContractName = "go.dedis.ch/dela.Evoting"

	// CmdArg is the argument's name to indicate the kind of command we want to
	// run on the contract. Should be one of the Command type.
	CmdArg = "evoting:command"

	CreateElectionArg = "evoting:createElectionArgs"

	CastVoteArg = "evoting:castVoteArgs"

	CancelElectionArg = "evoting:cancelElectionArgs"

	CloseElectionArg = "evoting:closeElectionArgs"

	ShuffleBallotsArg = "evoting:shuffleBallotsArgs"

	DecryptBallotsArg = "evoting:decryptBallotsArgs"

	// credentialAllCommand defines the credential command that is allowed to
	// perform all commands.
	credentialAllCommand = "all"
)

// Command defines a type of command for the value contract
type Command string

const (
	CmdCreateElection Command = "CREATE_ELECTION"

	CmdCastVote Command = "CAST_VOTE"

	CmdCloseElection Command = "CLOSE_ELECTION"

	CmdShuffleBallots Command = "SHUFFLE_BALLOTS"

	CmdDecryptBallots Command = "DECRYPT_BALLOTS"

	CmdCancelElection Command = "CANCEL_ELECTION"
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

	// access is the access control service managing this smart contract
	access access.Service

	// accessKey is the access identifier allowed to use this smart contract
	accessKey []byte

	// cmd provides the commands that can be executed by this smart contract
	cmd commands
}

// NewContract creates a new Value contract
func NewContract(aKey []byte, srvc access.Service) Contract {
	contract := Contract{
		// indexElection:     map[string]struct{}{},
		access:    srvc,
		accessKey: aKey,
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
	case CmdCreateElection:
		err := c.cmd.createElection(snap, step)
		if err != nil {
			return xerrors.Errorf("failed to CREATE ELECTION: %v", err)
		}
	case CmdCastVote:
		err := c.cmd.castVote(snap, step)
		if err != nil {
			return xerrors.Errorf("failed to CAST VOTE: %v", err)
		}
	case CmdCloseElection:
		err := c.cmd.closeElection(snap, step)
		if err != nil {
			return xerrors.Errorf("failed to CLOSE ELECTION: %v", err)
		}
	case CmdShuffleBallots:
		err := c.cmd.shuffleBallots(snap, step)
		if err != nil {
			return xerrors.Errorf("failed to SHUFFLE BALLOTS: %v", err)
		}
	case CmdDecryptBallots:
		err := c.cmd.decryptBallots(snap, step)
		if err != nil {
			return xerrors.Errorf("failed to DECRYPT BALLOTS: %v", err)
		}
	case CmdCancelElection:
		err := c.cmd.cancelElection(snap, step)
		if err != nil {
			return xerrors.Errorf("failed to CANCEL ELECTION: %v", err)
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

func (e evotingCommand) closeElection(snap store.Snapshot, step execution.Step) error {
	closeElectionArg := step.Current.GetArg(CloseElectionArg)
	if len(closeElectionArg) == 0 {
		return xerrors.Errorf("'%s' not found in tx arg", CloseElectionArg)
	}

	closeElectionTransaction := new(types.CloseElectionTransaction)
	err := json.NewDecoder(bytes.NewBuffer(closeElectionArg)).Decode(closeElectionTransaction)
	if err != nil {
		return xerrors.Errorf("failed to unmarshal CloseElectionTransaction: %v", err)
	}

	electionTxIDBuff, _ := hex.DecodeString(closeElectionTransaction.ElectionID)

	electionMarshalled, err := snap.Get(electionTxIDBuff)
	if err != nil {
		return xerrors.Errorf("failed to get key '%s': %v", electionTxIDBuff, err)
	}

	election := new(types.Election)
	err = json.NewDecoder(bytes.NewBuffer(electionMarshalled)).Decode(election)
	if err != nil {
		return xerrors.Errorf("failed to unmarshal Election: %v", err)
	}

	if election.AdminId != closeElectionTransaction.UserId {
		return xerrors.Errorf("only the admin can close the election")
	}

	if election.Status != types.Open {
		// todo : send status ?
		return xerrors.Errorf("the election is not open")
	}

	if len(election.EncryptedBallots) <= 1 {
		return xerrors.Errorf("at least two ballots are required")
	}

	election.Status = types.Closed

	js, err := json.Marshal(election)
	if err != nil {
		return xerrors.Errorf("failed to marshal Election: %v", err)
	}

	electionIDBuff, _ := hex.DecodeString(string(election.ElectionID))

	err = snap.Set(electionIDBuff, js)
	if err != nil {
		return xerrors.Errorf("failed to set value: %v", err)
	}

	return nil
}

func (e evotingCommand) shuffleBallots(snap store.Snapshot, step execution.Step) error {

	dela.Logger.Info().Msg("--------------------------------------SHUFFLE TRANSACTION START...")
	shuffleBallotsArg := step.Current.GetArg(ShuffleBallotsArg)
	if len(shuffleBallotsArg) == 0 {
		return xerrors.Errorf("'%s' not found in tx arg", ShuffleBallotsArg)
	}

	shuffleBallotsTransaction := new(types.ShuffleBallotsTransaction)
	err := json.NewDecoder(bytes.NewBuffer(shuffleBallotsArg)).Decode(shuffleBallotsTransaction)
	if err != nil {
		return xerrors.Errorf("failed to unmarshal ShuffleBallotsTransaction: %v", err)
	}

	// todo : implement test for this
	for _, tx := range step.Previous {

		if string(tx.GetArg(native.ContractArg)) == ContractName {

			if string(tx.GetArg("evoting:command")) == "evoting:command" {

				shuffleBallotsArgTx := tx.GetArg(ShuffleBallotsArg)
				shuffleBallotsTransactionTx := new(types.ShuffleBallotsTransaction)
				err := json.NewDecoder(bytes.NewBuffer(shuffleBallotsArgTx)).Decode(shuffleBallotsTransactionTx)

				if err != nil {
					return xerrors.Errorf("failed to unmarshall ShuffleBallotsTransaction : %v", err)
				}

				if shuffleBallotsTransactionTx.Round == shuffleBallotsTransaction.Round {
					return xerrors.Errorf(messageOnlyOneShufflePerRound)
				}
			}
		}
	}

	electionTxIDBuff, _ := hex.DecodeString(shuffleBallotsTransaction.ElectionID)

	electionMarshalled, err := snap.Get(electionTxIDBuff)
	if err != nil {
		return xerrors.Errorf("failed to get key '%s': %v", electionTxIDBuff, err)
	}

	election := new(types.Election)
	err = json.NewDecoder(bytes.NewBuffer(electionMarshalled)).Decode(election)
	if err != nil {
		return xerrors.Errorf("failed to unmarshal Election : %v", err)
	}

	if election.Status != types.Closed {
		// todo : send status ?
		return xerrors.Errorf("the election is not closed")
	}

	if len(election.ShuffledBallots) != shuffleBallotsTransaction.Round-1 {
		return xerrors.Errorf(messageOnlyOneShufflePerRound)
	}

	KsShuffled := make([]kyber.Point, 0, len(shuffleBallotsTransaction.ShuffledBallots))
	CsShuffled := make([]kyber.Point, 0, len(shuffleBallotsTransaction.ShuffledBallots))
	for _, v := range shuffleBallotsTransaction.ShuffledBallots {
		ciphertext := new(types.Ciphertext)
		err = json.NewDecoder(bytes.NewBuffer(v)).Decode(ciphertext)
		if err != nil {
			return xerrors.Errorf("failed to unmarshal Ciphertext: %v", err)
		}

		K := suite.Point()
		err = K.UnmarshalBinary(ciphertext.K)
		if err != nil {
			return xerrors.Errorf("failed to unmarshal K kyber.Point: %v", err)
		}

		C := suite.Point()
		err = C.UnmarshalBinary(ciphertext.C)
		if err != nil {
			return xerrors.Errorf("failed to unmarshal C kyber.Point: %v", err)
		}

		KsShuffled = append(KsShuffled, K)
		CsShuffled = append(CsShuffled, C)

	}

	pubKey := suite.Point()
	err = pubKey.UnmarshalBinary(election.Pubkey)
	if err != nil {
		return xerrors.Errorf("failed to unmarshal public key: %v", err)
	}

	Ks := make([]kyber.Point, 0, len(KsShuffled))
	Cs := make([]kyber.Point, 0, len(CsShuffled))

	encryptedBallotsMap := election.EncryptedBallots

	encryptedBallots := make([][]byte, 0, len(encryptedBallotsMap))

	if shuffleBallotsTransaction.Round == 1 {
		for _, value := range encryptedBallotsMap {
			encryptedBallots = append(encryptedBallots, value)
		}
	}

	if shuffleBallotsTransaction.Round > 1 {
		encryptedBallots = election.ShuffledBallots[shuffleBallotsTransaction.Round-1]
	}

	for _, v := range encryptedBallots {
		ciphertext := new(types.Ciphertext)
		err = json.NewDecoder(bytes.NewBuffer(v)).Decode(ciphertext)
		if err != nil {
			return xerrors.Errorf("failed to unmarshal Ciphertext: %v", err)
		}

		K := suite.Point()
		err = K.UnmarshalBinary(ciphertext.K)
		if err != nil {
			return xerrors.Errorf("failed to unmarshal K kyber.Point: %v", err)
		}

		C := suite.Point()
		err = C.UnmarshalBinary(ciphertext.C)
		if err != nil {
			return xerrors.Errorf("failed to unmarshal C kyber.Point: %v", err)
		}

		Ks = append(Ks, K)
		Cs = append(Cs, C)
	}

	// todo: add trusted nodes in election struct
	verifier := shuffleKyber.Verifier(suite, nil, pubKey, Ks, Cs, KsShuffled, CsShuffled)

	/*
		fmt.Printf(" KS : %v", Ks)
		fmt.Printf(" CS : %v", Cs)
		fmt.Printf(" KsShuffled : %v", KsShuffled)
		fmt.Printf(" CsShuffled : %v", CsShuffled)
	*/

	err = proof.HashVerify(suite, protocolName, verifier, shuffleBallotsTransaction.Proof)
	if err != nil {
		dela.Logger.Info().Msg("PROOF FAILED !!!!!!!!" + err.Error())
		// return xerrors.Errorf("proof verification failed: %v", err)
	}

	// todo : threshold should be part of election struct
	if shuffleBallotsTransaction.Round == 3 {
		election.Status = types.ShuffledBallots
	}

	election.ShuffledBallots[shuffleBallotsTransaction.Round] = shuffleBallotsTransaction.ShuffledBallots
	election.Proofs[shuffleBallotsTransaction.Round] = shuffleBallotsTransaction.Proof

	js, err := json.Marshal(election)
	if err != nil {
		return xerrors.Errorf("failed to marshall Election : %v", err)
	}

	electionIDBuff, _ := hex.DecodeString(string(election.ElectionID))

	err = snap.Set(electionIDBuff, js)
	if err != nil {
		return xerrors.Errorf("failed to set value: %v", err)
	}

	return nil
}

func (e evotingCommand) decryptBallots(snap store.Snapshot, step execution.Step) error {
	decryptBallotsArg := step.Current.GetArg(DecryptBallotsArg)
	if len(decryptBallotsArg) == 0 {
		return xerrors.Errorf("'%s' not found in tx arg", DecryptBallotsArg)
	}

	decryptBallotsTransaction := new(types.DecryptBallotsTransaction)
	err := json.NewDecoder(bytes.NewBuffer(decryptBallotsArg)).Decode(decryptBallotsTransaction)
	if err != nil {
		return xerrors.Errorf("failed to unmarshal DecryptBallotsTransaction: %v", err)
	}

	electionTxIDBuff, _ := hex.DecodeString(decryptBallotsTransaction.ElectionID)

	electionMarshalled, err := snap.Get(electionTxIDBuff)
	if err != nil {
		return xerrors.Errorf("failed to get key '%s': %v", electionTxIDBuff, err)
	}

	election := new(types.Election)
	err = json.NewDecoder(bytes.NewBuffer(electionMarshalled)).Decode(election)
	if err != nil {
		return xerrors.Errorf("failed to unmarshal Election : %v", err)
	}

	if election.AdminId != decryptBallotsTransaction.UserId {
		return xerrors.Errorf("only the admin can decrypt the ballots")
	}

	if election.Status != types.ShuffledBallots {
		// todo : send status ?
		return xerrors.Errorf("the ballots are not shuffled")
	}

	election.Status = types.ResultAvailable
	election.DecryptedBallots = decryptBallotsTransaction.DecryptedBallots

	js, err := json.Marshal(election)
	if err != nil {
		return xerrors.Errorf("failed to marshall Election : %v", err)
	}

	electionIDBuff, _ := hex.DecodeString(string(election.ElectionID))

	err = snap.Set(electionIDBuff, js)
	if err != nil {
		return xerrors.Errorf("failed to set value: %v", err)
	}

	return nil
}

func (e evotingCommand) cancelElection(snap store.Snapshot, step execution.Step) error {
	cancelElectionArg := step.Current.GetArg(CancelElectionArg)
	if len(cancelElectionArg) == 0 {
		return xerrors.Errorf("'%s' not found in tx arg", CancelElectionArg)
	}

	cancelElectionTransaction := new(types.CancelElectionTransaction)
	err := json.NewDecoder(bytes.NewBuffer(cancelElectionArg)).Decode(cancelElectionTransaction)
	if err != nil {
		return xerrors.Errorf("failed to unmarshal CancelElectionTransaction: %v", err)
	}

	electionTxIDBuff, _ := hex.DecodeString(cancelElectionTransaction.ElectionID)

	electionMarshalled, err := snap.Get(electionTxIDBuff)
	if err != nil {
		return xerrors.Errorf("failed to get key '%s': %v", electionTxIDBuff, err)
	}

	election := new(types.Election)
	err = json.NewDecoder(bytes.NewBuffer(electionMarshalled)).Decode(election)
	if err != nil {
		return xerrors.Errorf("failed to unmarshal Election : %v", err)
	}

	if election.AdminId != cancelElectionTransaction.UserId {
		return xerrors.Errorf("only the admin can cancel the election")
	}

	election.Status = types.Canceled

	js, err := json.Marshal(election)
	if err != nil {
		return xerrors.Errorf("failed to marshal Election : %v", err)
	}

	electionIDBuff, _ := hex.DecodeString(string(election.ElectionID))

	err = snap.Set(electionIDBuff, js)
	if err != nil {
		return xerrors.Errorf("failed to set value: %v", err)
	}

	return nil
}

func (e evotingCommand) castVote(snap store.Snapshot, step execution.Step) error {
	castVoteArg := step.Current.GetArg(CastVoteArg)
	if len(castVoteArg) == 0 {
		return xerrors.Errorf("'%s' not found in tx arg", CastVoteArg)
	}

	castVoteTransaction := new(types.CastVoteTransaction)
	err := json.NewDecoder(bytes.NewBuffer(castVoteArg)).Decode(castVoteTransaction)
	if err != nil {
		return xerrors.Errorf("failed to unmarshal CastVoteTransaction: %v", err)
	}

	electionTxIDBuff, _ := hex.DecodeString(castVoteTransaction.ElectionID)

	electionMarshalled, err := snap.Get(electionTxIDBuff)
	if err != nil {
		return xerrors.Errorf("failed to get key '%s': %v", electionTxIDBuff, err)
	}

	election := new(types.Election)
	err = json.NewDecoder(bytes.NewBuffer(electionMarshalled)).Decode(election)
	if err != nil {
		return xerrors.Errorf("failed to unmarshal Election: %v", err)
	}

	if election.Status != types.Open {
		// todo : send status ?
		return xerrors.Errorf("the election is not open")
	}

	election.EncryptedBallots[castVoteTransaction.UserId] = castVoteTransaction.Ballot

	js, err := json.Marshal(election)
	if err != nil {
		return xerrors.Errorf("failed to marshal Election : %v", err)
	}

	electionIDBuff, _ := hex.DecodeString(string(election.ElectionID))

	err = snap.Set(electionIDBuff, js)
	if err != nil {
		return xerrors.Errorf("failed to set value: %v", err)
	}

	return nil

}

func (e evotingCommand) createElection(snap store.Snapshot, step execution.Step) error {
	createElectionArg := step.Current.GetArg(CreateElectionArg)
	if len(createElectionArg) == 0 {
		return xerrors.Errorf("'%s' not found in tx arg", CreateElectionArg)
	}

	createElectionTransaction := new(types.CreateElectionTransaction)
	err := json.NewDecoder(bytes.NewBuffer(createElectionArg)).Decode(createElectionTransaction)
	if err != nil {
		return xerrors.Errorf("failed to unmarshal CreateElectionTransaction : %v", err)
	}

	election := types.Election{
		Title:            createElectionTransaction.Title,
		ElectionID:       types.ID(createElectionTransaction.ElectionID),
		AdminId:          createElectionTransaction.AdminId,
		Candidates:       createElectionTransaction.Candidates,
		Status:           types.Open,
		Pubkey:           createElectionTransaction.PublicKey,
		EncryptedBallots: map[string][]byte{},
		ShuffledBallots:  map[int][][]byte{},
		Proofs:           map[int][]byte{},
		DecryptedBallots: []types.Ballot{},
	}

	js, err := json.Marshal(election)
	if err != nil {
		return xerrors.Errorf("failed to marshal Election : %v", err)
	}

	electionIDBuff, _ := hex.DecodeString(string(election.ElectionID))

	err = snap.Set(electionIDBuff, js)
	if err != nil {
		return xerrors.Errorf("failed to set value: %v", err)
	}

	return nil
}
