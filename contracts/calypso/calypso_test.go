package calypso

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/execution/native"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/signed"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestExecute(t *testing.T) {
	contract := NewContract([]byte{}, fakeAccess{err: fake.GetError()})

	err := contract.Execute(fakeStore{}, makeStep(t))
	require.EqualError(t, err, "identity not authorized: fake.PublicKey ("+fake.GetError().Error()+")")

	contract = NewContract([]byte{}, fakeAccess{})
	err = contract.Execute(fakeStore{}, makeStep(t))
	require.EqualError(t, err, "'calypso:command' not found in tx arg")

	contract.cmd = fakeCmd{err: fake.GetError()}

	err = contract.Execute(fakeStore{}, makeStep(t, CmdArg, "ADVERTISE_SMC"))
	require.EqualError(t, err, fake.Err("failed to ADVERTISE_SMC"))

	err = contract.Execute(fakeStore{}, makeStep(t, CmdArg, "DELETE_SMC"))
	require.EqualError(t, err, fake.Err("failed to DELETE_SMC"))

	err = contract.Execute(fakeStore{}, makeStep(t, CmdArg, "LIST_SMC"))
	require.EqualError(t, err, fake.Err("failed to LIST_SMC"))

	err = contract.Execute(fakeStore{}, makeStep(t, CmdArg, "fake"))
	require.EqualError(t, err, "unknown command: fake")

	contract.cmd = fakeCmd{}
	err = contract.Execute(fakeStore{}, makeStep(t, CmdArg, "ADVERTISE_SMC"))
	require.NoError(t, err)
}

func TestCommand_AdvertiseSmc(t *testing.T) {
	contract := NewContract([]byte{}, fakeAccess{})

	cmd := calypsoCommand{
		Contract: &contract,
	}

	err := cmd.advertiseSmc(fake.NewSnapshot(), makeStep(t))
	require.EqualError(t, err, "'calypso:public_key' not found in tx arg")

	err = cmd.advertiseSmc(fake.NewSnapshot(), makeStep(t, KeyArg, "dummy"))
	require.EqualError(t, err, "'calypso:roster' not found in tx arg")

	err = cmd.advertiseSmc(fake.NewBadSnapshot(), makeStep(t, KeyArg, "dummy", RosterArg, "node:12345"))
	require.EqualError(t, err, fake.Err("failed to set roster"))

	err = cmd.advertiseSmc(fake.NewBadSnapshot(), makeStep(t, KeyArg, "dummy", RosterArg, ","))
	require.ErrorContains(t, err, "invalid node '' in roster")

	err = cmd.advertiseSmc(fake.NewBadSnapshot(), makeStep(t, KeyArg, "dummy", RosterArg, "abcd"))
	require.ErrorContains(t, err, "invalid node 'abcd' in roster")

	snap := fake.NewSnapshot()

	_, found := contract.index["dummy"]
	require.False(t, found)

	err = cmd.advertiseSmc(snap, makeStep(t, KeyArg, "dummy", RosterArg, "node:12345"))
	require.NoError(t, err)

	_, found = contract.index["dummy"]
	require.True(t, found)

	res, err := snap.Get([]byte("dummy"))
	require.NoError(t, err)
	require.Equal(t, "node:12345", string(res))
}

func TestCommand_DeleteSmc(t *testing.T) {
	contract := NewContract([]byte{}, fakeAccess{})

	cmd := calypsoCommand{
		Contract: &contract,
	}

	key := []byte("dummy")
	keyHex := hex.EncodeToString(key)
	keyStr := string(key)

	err := cmd.deleteSmc(fake.NewSnapshot(), makeStep(t))
	require.EqualError(t, err, "'calypso:public_key' not found in tx arg")

	err = cmd.deleteSmc(fake.NewBadSnapshot(), makeStep(t, KeyArg, keyStr))
	require.EqualError(t, err, fake.Err("failed to deleteSmc key '"+keyHex+"'"))

	snap := fake.NewSnapshot()
	snap.Set(key, []byte("localhost:12345"))
	contract.index[keyStr] = struct{}{}

	err = cmd.deleteSmc(snap, makeStep(t, KeyArg, keyStr))
	require.NoError(t, err)

	res, err := snap.Get(key)
	require.Nil(t, err)
	require.Nil(t, res)

	_, found := contract.index[keyStr]
	require.False(t, found)
}

func TestCommand_ListSmc(t *testing.T) {
	contract := NewContract([]byte{}, fakeAccess{})

	key1 := "key1"
	key2 := "key2"

	contract.index[key1] = struct{}{}
	contract.index[key2] = struct{}{}

	buf := &bytes.Buffer{}
	contract.printer = buf

	cmd := calypsoCommand{
		Contract: &contract,
	}

	snap := fake.NewSnapshot()
	snap.Set([]byte(key1), []byte("localhost:12345"))
	snap.Set([]byte(key2), []byte("localhost:12345,remote:54321"))

	err := cmd.listSmc(snap)
	require.NoError(t, err)

	require.Equal(t, fmt.Sprintf("%x=localhost:12345,%x=localhost:12345,remote:54321", key1, key2), buf.String())

	err = cmd.listSmc(fake.NewBadSnapshot())
	// we can't assume an order from the map
	require.Regexp(t, "^failed to get key", err.Error())
}

func TestInfoLog(t *testing.T) {
	log := infoLog{}

	n, err := log.Write([]byte{0b0, 0b1})
	require.NoError(t, err)
	require.Equal(t, 2, n)
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

func (c fakeCmd) advertiseSmc(snap store.Snapshot, step execution.Step) error {
	return c.err
}

func (c fakeCmd) deleteSmc(snap store.Snapshot, step execution.Step) error {
	return c.err
}

func (c fakeCmd) listSmc(snap store.Snapshot) error {
	return c.err
}
