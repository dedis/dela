package evoting

import (
	"bytes"
	"encoding/json"
	"go.dedis.ch/dela"
	"go.dedis.ch/dela/contracts/evoting/types"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/execution/native"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/kyber/v3"
	//shuffleKyber "go.dedis.ch/kyber/v3/shuffle"
	"go.dedis.ch/kyber/v3/suites"
	"golang.org/x/xerrors"
	"io"
	"net/http"
	"strconv"
)

const url = "http://localhost:"
const getPublicKeyEndPoint = "/dkg/pubkey"
const encryptEndPoint = "/dkg/encrypt"
const decryptEndPoint = "/dkg/decrypt"
var suite = suites.MustFind("Ed25519")


// commands defines the commands of the evoting contract. This interface helps in
// testing the contract.
type commands interface {
	getPublicKey(snap store.Snapshot, step execution.Step) error
	encrypt(snap store.Snapshot, step execution.Step) error
	decrypt(snap store.Snapshot, step execution.Step) error
	createSimpleElection(snap store.Snapshot, step execution.Step) error
	castVote(snap store.Snapshot, step execution.Step) error
	closeElection(snap store.Snapshot, step execution.Step) error
	shuffleBallots(snap store.Snapshot, step execution.Step) error
	decryptBallots(snap store.Snapshot, step execution.Step) error
	cancelElection(snap store.Snapshot, step execution.Step) error
}

const (
	// ContractName is the name of the contract.
	ContractName = "go.dedis.ch/dela.Evoting"

	// PortNumberArg is the argument's name in the transaction that contains the
	// port number of the dkg http server you want to communicate with.
	PortNumberArg = "evoting:portNumber"

	// KeyArg is the argument's name in the transaction that contains the
	// key of the ciphertext.
	KeyArg = "evoting:key"

	// CmdArg is the argument's name to indicate the kind of command we want to
	// run on the contract. Should be one of the Command type.
	CmdArg = "evoting:command"

	CreateSimpleElectionArg = "evoting:simpleElectionArgs"

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
	/* enter this command to allow the use of the Evoting contract :
	memcoin --config /tmp/node1 pool add\
	    --key private.key\
	    --args go.dedis.ch/dela.ContractArg --args go.dedis.ch/dela.Access\
	    --args access:grant_id --args 0300000000000000000000000000000000000000000000000000000000000000\
	    --args access:grant_contract --args go.dedis.ch/dela.Evoting\
	    --args access:grant_command --args all\
	    --args access:identity --args $(crypto bls signer read --path private.key --format BASE64_PUBKEY)\
	    --args access:command --args GRANT
	 */

	// CmdGetPublicKey defines the command to init the DKG protocol and generate the public key
	/*
	memcoin --config /tmp/node1 pool add\
	    --key private.key\
	    --args go.dedis.ch/dela.ContractArg --args go.dedis.ch/dela.Evoting\
	    --args evoting:command --args GETPUBLICKEY --args evoting:portNumber --args 8080
	 */
	CmdGetPublicKey Command = "GETPUBLICKEY"

	// This command helps in testing, voters would send already encrypted ballots.
	// TODO : convert to a Store like command
	// CmdEncrypt defines the command store the encryption of a random plaintext, this plaintext is encrypted only once
	// in the server since its encryption is non-deterministic and the storing of the ciphertext would result in a
	// mismatch tree root error
	/*
	memcoin --config /tmp/node1 pool add\
	    --key private.key\
	    --args go.dedis.ch/dela.ContractArg --args go.dedis.ch/dela.Evoting\
	    --args evoting:command --args ENCRYPT --args evoting:portNumber --args 8080 --args evoting:key --args ciphertext1
	 */
	CmdEncrypt Command = "ENCRYPT"

	// CmdDecrypt defines the command to decrypt a value and print it
	/*
	memcoin --config /tmp/node1 pool add\
	    --key private.key\
	    --args go.dedis.ch/dela.ContractArg --args go.dedis.ch/dela.Evoting\
	    --args evoting:command --args DECRYPT --args evoting:portNumber --args 8080 --args evoting:key --args ciphertext1
	 */
	CmdDecrypt Command = "DECRYPT"

	CmdCreateSimpleElection Command = "CREATE_SIMPLE_ELECTION"

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
	// todo : do we really need this ?
	//indexElection map[string]struct{}

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
		//indexElection:     map[string]struct{}{},
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
	case CmdGetPublicKey:
		err := c.cmd.getPublicKey(snap, step)
		if err != nil {
			return xerrors.Errorf("failed to GETPUBLICKEY: %v", err)
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
	case CmdCreateSimpleElection:
		err := c.cmd.createSimpleElection(snap, step)
		if err != nil {
			return xerrors.Errorf("failed to CREATE SIMPLE ELECTION: %v", err)
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

type CloseElectionTransaction struct {
	ElectionID uint16
	UserId string
}

func (e evotingCommand) closeElection(snap store.Snapshot, step execution.Step) error {
	closeElectionArg := step.Current.GetArg(CloseElectionArg)

	closeElectionTransaction := new (CloseElectionTransaction)
	err := json.NewDecoder(bytes.NewBuffer(closeElectionArg)).Decode(closeElectionTransaction)
	if err != nil {
		return xerrors.Errorf("failed to set unmarshall CloseElectionTransaction : %v", err)
	}

	// todo : method to cast election id to bytes or even change type of election id
	electionMarshalled, err := snap.Get([]byte(strconv.Itoa(int(closeElectionTransaction.ElectionID))))
	if err != nil {
		return xerrors.Errorf("failed to get key '%s': %v", []byte(strconv.Itoa(int(closeElectionTransaction.ElectionID))), err)
	}

	simpleElection := new (types.SimpleElection)
	err = json.NewDecoder(bytes.NewBuffer(electionMarshalled)).Decode(simpleElection)
	if err != nil {
		return xerrors.Errorf("failed to set unmarshall SimpleElection : %v", err)
	}

	if simpleElection.AdminId != closeElectionTransaction.UserId{
		return xerrors.Errorf("Only the admin can close the election")
	}

	if simpleElection.Status != types.Open{
		// todo : send status ?
		return xerrors.Errorf("The election is not open.")
	}

	simpleElection.Status = types.Closed

	js, err := json.Marshal(simpleElection)
	if err != nil {
		return xerrors.Errorf("failed to set marshall types.SimpleElection : %v", err)
	}

	err = snap.Set([]byte(strconv.Itoa(int(simpleElection.ElectionID))), js)
	if err != nil {
		return xerrors.Errorf("failed to set value: %v", err)
	}

	return nil
}

type ShuffleBallotsTransaction struct {
	ElectionID uint16
	UserId string
	ShuffledBallots  [][]byte
	Proof 			 []byte
}

type Ciphertext struct {
	K []byte
	C []byte
}

func (e evotingCommand) shuffleBallots(snap store.Snapshot, step execution.Step) error {
	shuffleBallotsArg := step.Current.GetArg(ShuffleBallotsArg)

	shuffleBallotsTransaction := new (ShuffleBallotsTransaction)
	err := json.NewDecoder(bytes.NewBuffer(shuffleBallotsArg)).Decode(shuffleBallotsTransaction)
	if err != nil {
		return xerrors.Errorf("failed to unmarshall ShuffleBallotsTransaction : %v", err)
	}

	// todo : method to cast election id to bytes or even change type of election id
	electionMarshalled, err := snap.Get([]byte(strconv.Itoa(int(shuffleBallotsTransaction.ElectionID))))
	if err != nil {
		return xerrors.Errorf("failed to get key '%s': %v", []byte(strconv.Itoa(int(shuffleBallotsTransaction.ElectionID))), err)
	}

	simpleElection := new (types.SimpleElection)
	err = json.NewDecoder(bytes.NewBuffer(electionMarshalled)).Decode(simpleElection)
	if err != nil {
		return xerrors.Errorf("failed to unmarshall SimpleElection : %v", err)
	}

	if simpleElection.AdminId != shuffleBallotsTransaction.UserId{
		return xerrors.Errorf("Only the admin can shuffle the ballots")
	}

	if simpleElection.Status != types.Closed{
		// todo : send status ?
		return xerrors.Errorf("The election is not closed.")
	}

	Ks := make([]kyber.Point, 0, len(shuffleBallotsTransaction.ShuffledBallots))
	Cs := make([]kyber.Point, 0, len(shuffleBallotsTransaction.ShuffledBallots))
	for _, v := range shuffleBallotsTransaction.ShuffledBallots{
		ciphertext:= new (Ciphertext)
		err = json.NewDecoder(bytes.NewBuffer(v)).Decode(ciphertext)
		if err != nil {
			return xerrors.Errorf("failed to unmarshall Ciphertext : %v", err)
		}

		K := suite.Point()
		err = K.UnmarshalBinary(ciphertext.K)
		if err != nil {
			return xerrors.Errorf("failed to unmarshall kyber.Point : %v", err)
		}

		C := suite.Point()
		err = C.UnmarshalBinary(ciphertext.C)
		if err != nil {
			return xerrors.Errorf("failed to unmarshall kyber.Point : %v", err)
		}

		Ks = append(Ks, K)
		Cs = append(Cs, C)

	}

	// todo : we need all shuffles and all proofs, ask Noémien and Gaurav!

	//verifier := shuffleKyber.Verifier(suite, nil, pubKey, Ks, Cs, KsShuffled, CsShuffled)

	simpleElection.Status = types.ShuffledBallots
	simpleElection.ShuffledBallots = shuffleBallotsTransaction.ShuffledBallots
	simpleElection.Proof = shuffleBallotsTransaction.Proof

	js, err := json.Marshal(simpleElection)
	if err != nil {
		return xerrors.Errorf("failed to marshall types.SimpleElection : %v", err)
	}

	err = snap.Set([]byte(strconv.Itoa(int(simpleElection.ElectionID))), js)
	if err != nil {
		return xerrors.Errorf("failed to set value: %v", err)
	}

	return nil
}

type DecryptBallotsTransaction struct {
	ElectionID uint16
	UserId string
	DecryptedBallots []types.SimpleBallot
}

func (e evotingCommand) decryptBallots(snap store.Snapshot, step execution.Step) error {
	decryptBallotsArg := step.Current.GetArg(DecryptBallotsArg)

	decryptBallotsTransaction := new (DecryptBallotsTransaction)
	err := json.NewDecoder(bytes.NewBuffer(decryptBallotsArg)).Decode(decryptBallotsTransaction)
	if err != nil {
		return xerrors.Errorf("failed to unmarshall DecryptBallotsTransaction : %v", err)
	}

	// todo : method to cast election id to bytes or even change type of election id
	electionMarshalled, err := snap.Get([]byte(strconv.Itoa(int(decryptBallotsTransaction.ElectionID))))
	if err != nil {
		return xerrors.Errorf("failed to get key '%s': %v", []byte(strconv.Itoa(int(decryptBallotsTransaction.ElectionID))), err)
	}

	simpleElection := new (types.SimpleElection)
	err = json.NewDecoder(bytes.NewBuffer(electionMarshalled)).Decode(simpleElection)
	if err != nil {
		return xerrors.Errorf("failed to unmarshall SimpleElection : %v", err)
	}

	if simpleElection.AdminId != decryptBallotsTransaction.UserId{
		return xerrors.Errorf("Only the admin can decrypt the ballots")
	}

	if simpleElection.Status != types.ShuffledBallots{
		// todo : send status ?
		return xerrors.Errorf("The ballots are not shuffled.")
	}

	simpleElection.Status = types.ResultAvailable
	simpleElection.DecryptedBallots = decryptBallotsTransaction.DecryptedBallots

	js, err := json.Marshal(simpleElection)
	if err != nil {
		return xerrors.Errorf("failed to marshall types.SimpleElection : %v", err)
	}

	err = snap.Set([]byte(strconv.Itoa(int(simpleElection.ElectionID))), js)
	if err != nil {
		return xerrors.Errorf("failed to set value: %v", err)
	}

	return nil
}

type CancelElectionTransaction struct {
	ElectionID uint16
	UserId string
}

func (e evotingCommand) cancelElection(snap store.Snapshot, step execution.Step) error {
	cancelElectionArg := step.Current.GetArg(CancelElectionArg)

	cancelElectionTransaction := new (CancelElectionTransaction)
	err := json.NewDecoder(bytes.NewBuffer(cancelElectionArg)).Decode(cancelElectionTransaction)
	if err != nil {
		return xerrors.Errorf("failed to set unmarshall CancelElectionTransaction : %v", err)
	}

	// todo : method to cast election id to bytes or even change type of election id
	electionMarshalled, err := snap.Get([]byte(strconv.Itoa(int(cancelElectionTransaction.ElectionID))))
	if err != nil {
		return xerrors.Errorf("failed to get key '%s': %v", []byte(strconv.Itoa(int(cancelElectionTransaction.ElectionID))), err)
	}

	simpleElection := new (types.SimpleElection)
	err = json.NewDecoder(bytes.NewBuffer(electionMarshalled)).Decode(simpleElection)
	if err != nil {
		return xerrors.Errorf("failed to set unmarshall SimpleElection : %v", err)
	}

	if simpleElection.AdminId != cancelElectionTransaction.UserId{
		return xerrors.Errorf("Only the admin can cancel the election")
	}

	simpleElection.Status = types.Canceled

	js, err := json.Marshal(simpleElection)
	if err != nil {
		return xerrors.Errorf("failed to set marshall types.SimpleElection : %v", err)
	}

	err = snap.Set([]byte(strconv.Itoa(int(simpleElection.ElectionID))), js)
	if err != nil {
		return xerrors.Errorf("failed to set value: %v", err)
	}

	return nil
}

type CastVoteTransaction struct {
	ElectionID uint16
	UserId string
	Ballot []byte
}

func (e evotingCommand) castVote(snap store.Snapshot, step execution.Step) error {

	// todo : check ballots in candidates

	castVoteArg := step.Current.GetArg(CastVoteArg)

	castVoteTransaction := new (CastVoteTransaction)
	err := json.NewDecoder(bytes.NewBuffer(castVoteArg)).Decode(castVoteTransaction)
	if err != nil {
		return xerrors.Errorf("failed to set unmarshall CastVoteTransaction : %v", err)
	}

	// todo : method to cast election id to bytes or even change type of election id
	electionMarshalled, err := snap.Get([]byte(strconv.Itoa(int(castVoteTransaction.ElectionID))))
	if err != nil {
		return xerrors.Errorf("failed to get key '%s': %v", []byte(strconv.Itoa(int(castVoteTransaction.ElectionID))), err)
	}

	simpleElection := new (types.SimpleElection)
	err = json.NewDecoder(bytes.NewBuffer(electionMarshalled)).Decode(simpleElection)
	if err != nil {
		return xerrors.Errorf("failed to set unmarshall SimpleElection : %v", err)
	}

	if simpleElection.Status != types.Open{
		// todo : send status ?
		return xerrors.Errorf("The election is not open.")
	}

	simpleElection.EncryptedBallots[castVoteTransaction.UserId] = castVoteTransaction.Ballot

	js, err := json.Marshal(simpleElection)
	if err != nil {
		return xerrors.Errorf("failed to set marshall types.SimpleElection : %v", err)
	}

	err = snap.Set([]byte(strconv.Itoa(int(simpleElection.ElectionID))), js)
	if err != nil {
		return xerrors.Errorf("failed to set value: %v", err)
	}

	return nil

}

type CreateSimpleElectionTransaction struct {
	ElectionID uint16
	Title string
	AdminId string
	Candidates []string
	PublicKey []byte
}

func (e evotingCommand) createSimpleElection(snap store.Snapshot, step execution.Step) error {
	createSimpleElectionArg := step.Current.GetArg(CreateSimpleElectionArg)

	createSimpleElectionTransaction := new (CreateSimpleElectionTransaction)
	err := json.NewDecoder(bytes.NewBuffer(createSimpleElectionArg)).Decode(createSimpleElectionTransaction)
	if err != nil {
		return xerrors.Errorf("failed to set unmarshall CreateSimpleElectionTransaction : %v", err)
	}
	
	simpleElection := types.SimpleElection{
		Title:            createSimpleElectionTransaction.Title,
		ElectionID:       types.ID(createSimpleElectionTransaction.ElectionID),
		AdminId:          createSimpleElectionTransaction.AdminId,
		Candidates:       createSimpleElectionTransaction.Candidates,
		Status:           types.Open,
		Pubkey:           createSimpleElectionTransaction.PublicKey,
		EncryptedBallots: map[string][]byte{},
		ShuffledBallots:  [][]byte{},
		DecryptedBallots: []types.SimpleBallot{},
	}

	js, err := json.Marshal(simpleElection)
	if err != nil {
		return xerrors.Errorf("failed to set marshall types.SimpleElection : %v", err)
	}

	err = snap.Set([]byte(strconv.Itoa(int(simpleElection.ElectionID))), js)
	if err != nil {
		return xerrors.Errorf("failed to set value: %v", err)
	}

	return nil
	
}

// getPublicKey implements commands. It performs the GETPUBLICKEY command
func (e evotingCommand) getPublicKey(snap store.Snapshot, step execution.Step) error {
	portNumber := step.Current.GetArg(PortNumberArg)

	resp, err := http.Get(url + string(portNumber) + getPublicKeyEndPoint)
	if err != nil {
		return xerrors.Errorf("failed to retrieve the public key: %v", err)
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)

	dela.Logger.Info().Msg("Response body : " + string(body))

	err = snap.Set([]byte("public_key"), body)
	if err != nil {
		return xerrors.Errorf("failed to set value: %v", err)
	}

	return nil
}

// encrypt implements commands. It performs the ENCRYPT command.
func (e evotingCommand) encrypt(snap store.Snapshot, step execution.Step) error {
	portNumber := step.Current.GetArg(PortNumberArg)
	keyArg := step.Current.GetArg(KeyArg)

	resp, err := http.Get(url + string(portNumber) + encryptEndPoint)
	if err != nil {
		return xerrors.Errorf("failed to retrieve the encryption from the server: %v", err)
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)

	dela.Logger.Info().Msg("Response body : " + string(body))

	err = snap.Set(keyArg, body)
	if err != nil {
		return xerrors.Errorf("failed to set value: %v", err)
	}

	return nil
}

// decrypt implements commands. It performs the DECRYPT command.
func (e evotingCommand) decrypt(snap store.Snapshot, step execution.Step) error {
	portNumber := step.Current.GetArg(PortNumberArg)
	keyArg := step.Current.GetArg(KeyArg)

	value, err := snap.Get(keyArg)
	if err != nil {
		return xerrors.Errorf("failed to get key '%s': %v", keyArg, err)
	}

	resp, err := http.Post(url + string(portNumber) + decryptEndPoint, "application/json", bytes.NewBuffer(value))
	if err != nil {
		return xerrors.Errorf("failed retrieve the decryption from the server: %v", err)
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)

	dela.Logger.Info().Msg("Response body : " + string(body))

	return nil
}

