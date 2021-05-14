package evoting

// todo: json marshall and unmarshall branch is are not covered yet

import (
	"bytes"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/contracts/evoting/types"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/execution/native"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/signed"
	"go.dedis.ch/dela/internal/testing/fake"
	"testing"
)

func TestExecute(t *testing.T) {
	contract := NewContract([]byte{}, fakeAccess{err: fake.GetError()})

	err := contract.Execute(fakeStore{}, makeStep(t))
	require.EqualError(t, err, "identity not authorized: fake.PublicKey ("+fake.GetError().Error()+")")

	contract = NewContract([]byte{}, fakeAccess{})
	err = contract.Execute(fakeStore{}, makeStep(t))
	require.EqualError(t, err, "'evoting:command' not found in tx arg")

	contract.cmd = fakeCmd{err: fake.GetError()}

	err = contract.Execute(fakeStore{}, makeStep(t, CmdArg, string(CmdCreateElection)))
	require.EqualError(t, err, fake.Err("failed to CREATE ELECTION"))

	err = contract.Execute(fakeStore{}, makeStep(t, CmdArg, string(CmdCastVote)))
	require.EqualError(t, err, fake.Err("failed to CAST VOTE"))

	err = contract.Execute(fakeStore{}, makeStep(t, CmdArg, string(CmdCloseElection)))
	require.EqualError(t, err, fake.Err("failed to CLOSE ELECTION"))

	err = contract.Execute(fakeStore{}, makeStep(t, CmdArg, string(CmdShuffleBallots)))
	require.EqualError(t, err, fake.Err("failed to SHUFFLE BALLOTS"))

	err = contract.Execute(fakeStore{}, makeStep(t, CmdArg, string(CmdDecryptBallots)))
	require.EqualError(t, err, fake.Err("failed to DECRYPT BALLOTS"))

	err = contract.Execute(fakeStore{}, makeStep(t, CmdArg, string(CmdCancelElection)))
	require.EqualError(t, err, fake.Err("failed to CANCEL ELECTION"))

	err = contract.Execute(fakeStore{}, makeStep(t, CmdArg, "fake"))
	require.EqualError(t, err, "unknown command: fake")

	contract.cmd = fakeCmd{}
	err = contract.Execute(fakeStore{}, makeStep(t, CmdArg, string(CmdCreateElection)))
	require.NoError(t, err)

}

func TestCommand_CreateElection(t *testing.T) {

	dummyCreateElectionTransaction := types.CreateElectionTransaction{
		ElectionID: "dummyId",
		Title:      "dummyTitle",
		AdminId:    "dummyAdminId",
		Candidates: []string{},
		PublicKey:  []byte{},
	}

	js, _ := json.Marshal(dummyCreateElectionTransaction)

	contract := NewContract([]byte{}, fakeAccess{})

	cmd := evotingCommand{
		Contract: &contract,
	}

	err := cmd.createElection(fake.NewSnapshot(), makeStep(t))
	require.EqualError(t, err, "'evoting:createElectionArgs' not found in tx arg")

	err = cmd.createElection(fake.NewSnapshot(), makeStep(t, CreateElectionArg, "dummy"))
	require.EqualError(t, err, "failed to unmarshal CreateElectionTransaction : "+
		"invalid character 'd' looking for beginning of value")

	err = cmd.createElection(fake.NewBadSnapshot(), makeStep(t, CreateElectionArg, string(js)))
	require.EqualError(t, err, fake.Err("failed to set value"))

	snap := fake.NewSnapshot()
	err = cmd.createElection(snap, makeStep(t, CreateElectionArg, string(js)))
	require.NoError(t, err)

	res, err := snap.Get([]byte("dummyId"))
	require.NoError(t, err)

	election := new(types.Election)
	_ = json.NewDecoder(bytes.NewBuffer(res)).Decode(election)

	require.Equal(t, dummyCreateElectionTransaction.ElectionID, string(election.ElectionID))
	require.Equal(t, dummyCreateElectionTransaction.Title, election.Title)
	require.Equal(t, dummyCreateElectionTransaction.AdminId, election.AdminId)
	require.Equal(t, dummyCreateElectionTransaction.Candidates, election.Candidates)
	require.Equal(t, dummyCreateElectionTransaction.PublicKey, election.Pubkey)
	require.Equal(t, types.Open, int(election.Status))

}

func TestCommand_CastVote(t *testing.T) {

	dummyCastVoteTransaction := types.CastVoteTransaction{
		ElectionID: "dummyId",
		UserId:     "dummyUserId",
		Ballot:     []byte{10},
	}
	jsCastVoteTransaction, _ := json.Marshal(dummyCastVoteTransaction)

	dummyElection := types.Election{
		Title:            "dummyTitle",
		ElectionID:       "dummyId",
		AdminId:          "dummyAdminId",
		Candidates:       nil,
		Status:           0,
		Pubkey:           nil,
		EncryptedBallots: map[string][]byte{},
		ShuffledBallots:  nil,
		Proofs:           nil,
		DecryptedBallots: nil,
		ShuffleThreshold: 0,
	}
	jsElection, _ := json.Marshal(dummyElection)

	contract := NewContract([]byte{}, fakeAccess{})

	cmd := evotingCommand{
		Contract: &contract,
	}

	err := cmd.castVote(fake.NewSnapshot(), makeStep(t))
	require.EqualError(t, err, "'evoting:castVoteArgs' not found in tx arg")

	err = cmd.castVote(fake.NewSnapshot(), makeStep(t, CastVoteArg, "dummy"))
	require.EqualError(t, err, "failed to unmarshal CastVoteTransaction: "+
		"invalid character 'd' looking for beginning of value")

	err = cmd.castVote(fake.NewBadSnapshot(), makeStep(t, CastVoteArg, string(jsCastVoteTransaction)))
	require.EqualError(t, err, fake.Err("failed to get key 'dummyId'"))

	snap := fake.NewSnapshot()
	_ = snap.Set([]byte("dummyId"), []byte("fake election"))
	err = cmd.castVote(snap, makeStep(t, CastVoteArg, string(jsCastVoteTransaction)))
	require.Contains(t, err.Error(), "failed to unmarshal Election")

	_ = snap.Set([]byte("dummyId"), jsElection)
	err = cmd.castVote(snap, makeStep(t, CastVoteArg, string(jsCastVoteTransaction)))
	require.EqualError(t, err, "the election is not open")

	dummyElection.Status = types.Open

	jsElection, _ = json.Marshal(dummyElection)

	_ = snap.Set([]byte("dummyId"), jsElection)
	err = cmd.castVote(snap, makeStep(t, CastVoteArg, string(jsCastVoteTransaction)))
	require.NoError(t, err)

	res, err := snap.Get([]byte("dummyId"))
	require.NoError(t, err)

	election := new(types.Election)
	_ = json.NewDecoder(bytes.NewBuffer(res)).Decode(election)

	require.Equal(t, dummyCastVoteTransaction.Ballot, election.EncryptedBallots[dummyCastVoteTransaction.UserId])
}

func TestCommand_CloseElection(t *testing.T) {

	dummyCloseElectionTransaction := types.CloseElectionTransaction{
		ElectionID: "dummyId",
		UserId:     "dummyUserId",
	}
	jsCloseElectionTransaction, _ := json.Marshal(dummyCloseElectionTransaction)

	dummyElection := types.Election{
		Title:            "dummyTitle",
		ElectionID:       "dummyId",
		AdminId:          "dummyAdminId",
		Candidates:       nil,
		Status:           0,
		Pubkey:           nil,
		EncryptedBallots: map[string][]byte{},
		ShuffledBallots:  nil,
		Proofs:           nil,
		DecryptedBallots: nil,
		ShuffleThreshold: 0,
	}
	jsElection, _ := json.Marshal(dummyElection)

	contract := NewContract([]byte{}, fakeAccess{})

	cmd := evotingCommand{
		Contract: &contract,
	}

	err := cmd.closeElection(fake.NewSnapshot(), makeStep(t))
	require.EqualError(t, err, "'evoting:closeElectionArgs' not found in tx arg")

	err = cmd.closeElection(fake.NewSnapshot(), makeStep(t, CloseElectionArg, "dummy"))
	require.EqualError(t, err, "failed to unmarshal CloseElectionTransaction: "+
		"invalid character 'd' looking for beginning of value")

	err = cmd.closeElection(fake.NewBadSnapshot(), makeStep(t, CloseElectionArg, string(jsCloseElectionTransaction)))
	require.EqualError(t, err, fake.Err("failed to get key 'dummyId'"))

	snap := fake.NewSnapshot()
	_ = snap.Set([]byte("dummyId"), []byte("fake election"))
	err = cmd.closeElection(snap, makeStep(t, CloseElectionArg, string(jsCloseElectionTransaction)))
	require.Contains(t, err.Error(), "failed to unmarshal Election")

	_ = snap.Set([]byte("dummyId"), jsElection)
	err = cmd.closeElection(snap, makeStep(t, CloseElectionArg, string(jsCloseElectionTransaction)))
	require.EqualError(t, err, "only the admin can close the election")

	dummyCloseElectionTransaction.UserId = "dummyAdminId"
	jsCloseElectionTransaction, _ = json.Marshal(dummyCloseElectionTransaction)
	err = cmd.closeElection(snap, makeStep(t, CloseElectionArg, string(jsCloseElectionTransaction)))
	require.EqualError(t, err, "the election is not open")

	dummyElection.Status = types.Open
	jsElection, _ = json.Marshal(dummyElection)
	_ = snap.Set([]byte("dummyId"), jsElection)

	err = cmd.closeElection(snap, makeStep(t, CloseElectionArg, string(jsCloseElectionTransaction)))
	require.NoError(t, err)

	res, err := snap.Get([]byte("dummyId"))
	require.NoError(t, err)

	election := new(types.Election)
	_ = json.NewDecoder(bytes.NewBuffer(res)).Decode(election)

	require.Equal(t, types.Closed, int(election.Status))

}

// todo : finish implementation of this test
func TestCommand_ShuffleBallots(t *testing.T) {

	dummyShuffleBallotsTransaction := types.ShuffleBallotsTransaction{
		ElectionID:      "dummyId",
		Round:           0,
		ShuffledBallots: nil,
		Proof:           nil,
	}
	jsShuffleBallotsTransaction, _ := json.Marshal(dummyShuffleBallotsTransaction)

	dummyElection := types.Election{
		Title:            "dummyTitle",
		ElectionID:       "dummyId",
		AdminId:          "dummyAdminId",
		Candidates:       nil,
		Status:           0,
		Pubkey:           nil,
		EncryptedBallots: map[string][]byte{},
		ShuffledBallots:  map[int][][]byte{},
		Proofs:           nil,
		DecryptedBallots: nil,
		ShuffleThreshold: 0,
	}
	jsElection, _ := json.Marshal(dummyElection)

	contract := NewContract([]byte{}, fakeAccess{})

	cmd := evotingCommand{
		Contract: &contract,
	}

	err := cmd.shuffleBallots(fake.NewSnapshot(), makeStep(t))
	require.EqualError(t, err, "'evoting:shuffleBallotsArgs' not found in tx arg")

	err = cmd.shuffleBallots(fake.NewSnapshot(), makeStep(t, ShuffleBallotsArg, "dummy"))
	require.EqualError(t, err, "failed to unmarshal ShuffleBallotsTransaction: "+
		"invalid character 'd' looking for beginning of value")

	err = cmd.shuffleBallots(fake.NewBadSnapshot(), makeStep(t, ShuffleBallotsArg, string(jsShuffleBallotsTransaction)))
	require.EqualError(t, err, fake.Err("failed to get key 'dummyId'"))

	snap := fake.NewSnapshot()
	_ = snap.Set([]byte("dummyId"), []byte("fake election"))
	err = cmd.shuffleBallots(snap, makeStep(t, ShuffleBallotsArg, string(jsShuffleBallotsTransaction)))
	require.Contains(t, err.Error(), "failed to unmarshal Election")

	_ = snap.Set([]byte("dummyId"), jsElection)
	err = cmd.shuffleBallots(snap, makeStep(t, ShuffleBallotsArg, string(jsShuffleBallotsTransaction)))
	require.EqualError(t, err, "the election is not closed")

	dummyElection.Status = types.Closed
	jsElection, _ = json.Marshal(dummyElection)
	_ = snap.Set([]byte("dummyId"), jsElection)
	err = cmd.shuffleBallots(snap, makeStep(t, ShuffleBallotsArg, string(jsShuffleBallotsTransaction)))
	require.EqualError(t, err, messageOnlyOneShufflePerRound)
}

func TestCommand_DecryptBallots(t *testing.T) {

	ballot1 := types.Ballot{Vote: "vote1"}
	ballot2 := types.Ballot{Vote: "vote2"}

	dummyDecryptBallotsTransaction := types.DecryptBallotsTransaction{
		ElectionID:       "dummyId",
		UserId:           "dummyUserId",
		DecryptedBallots: []types.Ballot{ballot1, ballot2},
	}
	jsDecryptBallotsTransaction, _ := json.Marshal(dummyDecryptBallotsTransaction)

	dummyElection := types.Election{
		Title:            "dummyTitle",
		ElectionID:       "dummyId",
		AdminId:          "dummyAdminId",
		Candidates:       nil,
		Status:           0,
		Pubkey:           nil,
		EncryptedBallots: map[string][]byte{},
		ShuffledBallots:  nil,
		Proofs:           nil,
		DecryptedBallots: []types.Ballot{},
		ShuffleThreshold: 0,
	}
	jsElection, _ := json.Marshal(dummyElection)

	contract := NewContract([]byte{}, fakeAccess{})

	cmd := evotingCommand{
		Contract: &contract,
	}

	err := cmd.decryptBallots(fake.NewSnapshot(), makeStep(t))
	require.EqualError(t, err, "'evoting:decryptBallotsArgs' not found in tx arg")

	err = cmd.decryptBallots(fake.NewSnapshot(), makeStep(t, DecryptBallotsArg, "dummy"))
	require.EqualError(t, err, "failed to unmarshal DecryptBallotsTransaction: "+
		"invalid character 'd' looking for beginning of value")

	err = cmd.decryptBallots(fake.NewBadSnapshot(), makeStep(t, DecryptBallotsArg, string(jsDecryptBallotsTransaction)))
	require.EqualError(t, err, fake.Err("failed to get key 'dummyId'"))

	snap := fake.NewSnapshot()
	_ = snap.Set([]byte("dummyId"), []byte("fake election"))
	err = cmd.decryptBallots(snap, makeStep(t, DecryptBallotsArg, string(jsDecryptBallotsTransaction)))
	require.Contains(t, err.Error(), "failed to unmarshal Election")

	_ = snap.Set([]byte("dummyId"), jsElection)
	err = cmd.decryptBallots(snap, makeStep(t, DecryptBallotsArg, string(jsDecryptBallotsTransaction)))
	require.EqualError(t, err, "only the admin can decrypt the ballots")

	dummyDecryptBallotsTransaction.UserId = "dummyAdminId"
	jsDecryptBallotsTransaction, _ = json.Marshal(dummyDecryptBallotsTransaction)
	err = cmd.decryptBallots(snap, makeStep(t, DecryptBallotsArg, string(jsDecryptBallotsTransaction)))
	require.EqualError(t, err, "the ballots are not shuffled")

	dummyElection.Status = types.ShuffledBallots

	jsElection, _ = json.Marshal(dummyElection)

	_ = snap.Set([]byte("dummyId"), jsElection)
	err = cmd.decryptBallots(snap, makeStep(t, DecryptBallotsArg, string(jsDecryptBallotsTransaction)))
	require.NoError(t, err)

	res, err := snap.Get([]byte("dummyId"))
	require.NoError(t, err)

	election := new(types.Election)
	_ = json.NewDecoder(bytes.NewBuffer(res)).Decode(election)

	require.Equal(t, dummyDecryptBallotsTransaction.DecryptedBallots, election.DecryptedBallots)
	require.Equal(t, types.ResultAvailable, int(election.Status))

}

func TestCommand_CancelElection(t *testing.T) {

	dummyCancelElectionTransaction := types.CancelElectionTransaction{
		ElectionID: "dummyId",
		UserId:     "dummyUserId",
	}
	jsCancelElectionTransaction, _ := json.Marshal(dummyCancelElectionTransaction)

	dummyElection := types.Election{
		Title:            "dummyTitle",
		ElectionID:       "dummyId",
		AdminId:          "dummyAdminId",
		Candidates:       nil,
		Status:           1,
		Pubkey:           nil,
		EncryptedBallots: map[string][]byte{},
		ShuffledBallots:  nil,
		Proofs:           nil,
		DecryptedBallots: nil,
		ShuffleThreshold: 0,
	}
	jsElection, _ := json.Marshal(dummyElection)

	contract := NewContract([]byte{}, fakeAccess{})

	cmd := evotingCommand{
		Contract: &contract,
	}

	err := cmd.cancelElection(fake.NewSnapshot(), makeStep(t))
	require.EqualError(t, err, "'evoting:cancelElectionArgs' not found in tx arg")

	err = cmd.cancelElection(fake.NewSnapshot(), makeStep(t, CancelElectionArg, "dummy"))
	require.EqualError(t, err, "failed to unmarshal CancelElectionTransaction: "+
		"invalid character 'd' looking for beginning of value")

	err = cmd.cancelElection(fake.NewBadSnapshot(), makeStep(t, CancelElectionArg, string(jsCancelElectionTransaction)))
	require.EqualError(t, err, fake.Err("failed to get key 'dummyId'"))

	snap := fake.NewSnapshot()
	_ = snap.Set([]byte("dummyId"), []byte("fake election"))
	err = cmd.cancelElection(snap, makeStep(t, CancelElectionArg, string(jsCancelElectionTransaction)))
	require.Contains(t, err.Error(), "failed to unmarshal Election")

	_ = snap.Set([]byte("dummyId"), jsElection)
	err = cmd.cancelElection(snap, makeStep(t, CancelElectionArg, string(jsCancelElectionTransaction)))
	require.EqualError(t, err, "only the admin can cancel the election")

	dummyCancelElectionTransaction.UserId = "dummyAdminId"
	jsCancelElectionTransaction, _ = json.Marshal(dummyCancelElectionTransaction)
	err = cmd.cancelElection(snap, makeStep(t, CancelElectionArg, string(jsCancelElectionTransaction)))
	require.NoError(t, err)

	res, err := snap.Get([]byte("dummyId"))
	require.NoError(t, err)

	election := new(types.Election)
	_ = json.NewDecoder(bytes.NewBuffer(res)).Decode(election)

	require.Equal(t, types.Canceled, int(election.Status))

}

func TestRegisterContract(t *testing.T) {
	RegisterContract(native.NewExecution(), Contract{})
}

// -----------------------------------------------------------------------------
// Utility functions

func makeStep(t *testing.T, args ...string) execution.Step {
	return execution.Step{Current: makeTx(t, args...)}
}

func makeTx(t *testing.T, args ...string) txn.Transaction {
	options := []signed.TransactionOption{}
	for i := 0; i < len(args)-1; i += 2 {
		options = append(options, signed.WithArg(args[i], []byte(args[i+1])))
	}

	tx, err := signed.NewTransaction(0, fake.PublicKey{}, options...)
	require.NoError(t, err)

	return tx
}

type fakeAccess struct {
	access.Service

	err error
}

func (srvc fakeAccess) Match(store.Readable, access.Credential, ...access.Identity) error {
	return srvc.err
}

func (srvc fakeAccess) Grant(store.Snapshot, access.Credential, ...access.Identity) error {
	return srvc.err
}

type fakeStore struct {
	store.Snapshot
}

func (s fakeStore) Get(key []byte) ([]byte, error) {
	return nil, nil
}

func (s fakeStore) Set(key, value []byte) error {
	return nil
}

type fakeCmd struct {
	err error
}

func (c fakeCmd) getPublicKey(snap store.Snapshot, step execution.Step) error {
	return c.err
}

func (c fakeCmd) encrypt(snap store.Snapshot, step execution.Step) error {
	return c.err
}

func (c fakeCmd) decrypt(snap store.Snapshot, step execution.Step) error {
	return c.err
}

func (c fakeCmd) createElection(snap store.Snapshot, step execution.Step) error {
	return c.err
}

func (c fakeCmd) castVote(snap store.Snapshot, step execution.Step) error {
	return c.err
}

func (c fakeCmd) closeElection(snap store.Snapshot, step execution.Step) error {
	return c.err
}

func (c fakeCmd) shuffleBallots(snap store.Snapshot, step execution.Step) error {
	return c.err
}

func (c fakeCmd) decryptBallots(snap store.Snapshot, step execution.Step) error {
	return c.err
}

func (c fakeCmd) cancelElection(snap store.Snapshot, step execution.Step) error {
	return c.err
}
