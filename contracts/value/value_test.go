package value

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
	"go.dedis.ch/dela/core/store/prefixed"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/signed"
	"go.dedis.ch/dela/testing/fake"
)

func TestExecute(t *testing.T) {
	contract := NewContract(fakeAccess{err: fake.GetError()})

	err := contract.Execute(fakeStore{}, makeStep(t))
	require.EqualError(t, err,
		"identity not authorized: fake.PublicKey ("+fake.GetError().Error()+")")

	contract = NewContract(fakeAccess{})
	err = contract.Execute(fakeStore{}, makeStep(t))
	require.EqualError(t, err, "'value:command' not found in tx arg")

	contract.cmd = fakeCmd{err: fake.GetError()}

	err = contract.Execute(fakeStore{}, makeStep(t, CmdArg, "WRITE"))
	require.EqualError(t, err, fake.Err("failed to WRITE"))

	err = contract.Execute(fakeStore{}, makeStep(t, CmdArg, "READ"))
	require.EqualError(t, err, fake.Err("failed to READ"))

	err = contract.Execute(fakeStore{}, makeStep(t, CmdArg, "DELETE"))
	require.EqualError(t, err, fake.Err("failed to DELETE"))

	err = contract.Execute(fakeStore{}, makeStep(t, CmdArg, "LIST"))
	require.EqualError(t, err, fake.Err("failed to LIST"))

	err = contract.Execute(fakeStore{}, makeStep(t, CmdArg, "fake"))
	require.EqualError(t, err, "unknown command: fake")

	contract.cmd = fakeCmd{}
	err = contract.Execute(fakeStore{}, makeStep(t, CmdArg, "WRITE"))
	require.NoError(t, err)
}

func TestCommand_Write(t *testing.T) {
	contract := NewContract(fakeAccess{})

	cmd := valueCommand{
		Contract: &contract,
	}

	snap := prefixed.NewSnapshot(ContractUID, fake.NewSnapshot())
	err := cmd.write(snap, makeStep(t))
	require.EqualError(t, err, "'value:key' not found in tx arg")

	snap = prefixed.NewSnapshot(ContractUID, fake.NewSnapshot())
	err = cmd.write(snap, makeStep(t, KeyArg, "dummy"))
	require.EqualError(t, err, "'value:value' not found in tx arg")

	snap = prefixed.NewSnapshot(ContractUID, fake.NewBadSnapshot())
	err = cmd.write(snap, makeStep(t, KeyArg, "dummy", ValueArg, "value"))
	require.EqualError(t, err, fake.Err("failed to set value"))

	_, found := contract.index["dummy"]
	require.False(t, found)

	snap = prefixed.NewSnapshot(ContractUID, fake.NewSnapshot())
	err = cmd.write(snap, makeStep(t, KeyArg, "dummy", ValueArg, "value"))
	require.NoError(t, err)

	_, found = contract.index["dummy"]
	require.True(t, found)

	res, err := snap.Get([]byte("dummy"))
	require.NoError(t, err)
	require.Equal(t, "value", string(res))
}

func TestCommand_Read(t *testing.T) {
	contract := NewContract(fakeAccess{})

	cmd := valueCommand{
		Contract: &contract,
	}

	key := []byte("dummy")
	keyHex := hex.EncodeToString(key)

	snap := prefixed.NewSnapshot(ContractUID, fake.NewSnapshot())
	err := cmd.read(snap, makeStep(t))
	require.EqualError(t, err, "'value:key' not found in tx arg")

	badSnap := prefixed.NewSnapshot(ContractUID, fake.NewBadSnapshot())
	err = cmd.read(badSnap, makeStep(t, KeyArg, "dummy"))
	require.EqualError(t, err, fake.Err("failed to get key 'dummy'"))

	snap = prefixed.NewSnapshot(ContractUID, fake.NewSnapshot())
	err = snap.Set(key, []byte("value"))
	require.NoError(t, err)

	buf := &bytes.Buffer{}
	cmd.Contract.printer = buf

	err = cmd.read(snap, makeStep(t, KeyArg, "dummy"))
	require.NoError(t, err)

	require.Equal(t, keyHex+"=value", buf.String())
}

func TestCommand_Delete(t *testing.T) {
	contract := NewContract(fakeAccess{})

	cmd := valueCommand{
		Contract: &contract,
	}

	key := []byte("dummy")
	keyHex := hex.EncodeToString(key)
	keyStr := string(key)

	snap := prefixed.NewSnapshot(ContractUID, fake.NewSnapshot())
	err := cmd.delete(snap, makeStep(t))
	require.EqualError(t, err, "'value:key' not found in tx arg")

	badSnap := prefixed.NewSnapshot(ContractUID, fake.NewBadSnapshot())
	err = cmd.delete(badSnap, makeStep(t, KeyArg, keyStr))
	require.EqualError(t, err, fake.Err("failed to delete key '"+keyHex+"'"))

	snap = prefixed.NewSnapshot(ContractUID, fake.NewSnapshot())
	err = snap.Set(key, []byte("value"))
	require.NoError(t, err)
	contract.index[keyStr] = struct{}{}

	err = cmd.delete(snap, makeStep(t, KeyArg, keyStr))
	require.NoError(t, err)

	res, err := snap.Get(key)
	require.Nil(t, err)
	require.Nil(t, res) // = "key not found"

	_, found := contract.index[keyStr]
	require.False(t, found)
}

func TestCommand_List(t *testing.T) {
	contract := NewContract(fakeAccess{})

	key1 := "key1"
	key2 := "key2"

	contract.index[key1] = struct{}{}
	contract.index[key2] = struct{}{}

	buf := &bytes.Buffer{}
	contract.printer = buf

	cmd := valueCommand{
		Contract: &contract,
	}

	snap := prefixed.NewSnapshot(ContractUID, fake.NewSnapshot())
	err := snap.Set([]byte(key1), []byte("value1"))
	require.NoError(t, err)
	err = snap.Set([]byte(key2), []byte("value2"))
	require.NoError(t, err)

	err = cmd.list(snap)
	require.NoError(t, err)

	require.Equal(t, fmt.Sprintf("%x=value1,%x=value2", key1, key2), buf.String())

	badSnap := prefixed.NewSnapshot(ContractUID, fake.NewBadSnapshot())
	err = cmd.list(badSnap)
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

func (s fakeStore) Get(_ []byte) ([]byte, error) {
	return nil, nil
}

func (s fakeStore) Set(_, _ []byte) error {
	return nil
}

type fakeCmd struct {
	err error
}

func (c fakeCmd) write(_ store.Snapshot, _ execution.Step) error {
	return c.err
}

func (c fakeCmd) read(_ store.Snapshot, _ execution.Step) error {
	return c.err
}

func (c fakeCmd) delete(_ store.Snapshot, _ execution.Step) error {
	return c.err
}

func (c fakeCmd) list(_ store.Snapshot) error {
	return c.err
}
