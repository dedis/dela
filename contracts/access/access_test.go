package access

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/execution/native"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/signed"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestExecute(t *testing.T) {
	contract := NewContract([]byte{}, fakeAccess{err: fake.GetError()}, fakeStore{})
	err := contract.Execute(fakeStore{}, makeStep(t, CmdArg, ""))
	require.EqualError(t, err, "identity not authorized: fake.PublicKey ("+fake.GetError().Error()+")")

	contract = NewContract([]byte{}, fakeAccess{}, fakeStore{})
	err = contract.Execute(fakeStore{}, makeStep(t, CmdArg, ""))
	require.EqualError(t, err, "'access:command' not found in tx arg")

	err = contract.Execute(fakeStore{}, makeStep(t, CmdArg, "fake"))
	require.EqualError(t, err, "access, unknown command: fake")

	err = contract.Execute(fakeStore{}, makeStep(t, CmdArg, string(CmdSet)))
	require.EqualError(t, err, "failed to SET: 'access:grant_id' not found in tx arg")

	signer := bls.NewSigner()
	buf, err := signer.GetPublicKey().MarshalBinary()
	require.NoError(t, err)
	id := base64.StdEncoding.EncodeToString(buf)
	err = contract.Execute(fakeStore{}, makeStep(t, CmdArg, string(CmdSet),
		GrantIDArg, "deadbeef",
		GrantContractArg, "fake contract",
		GrantCommandArg, "fake command",
		IdentityArg, id))
	require.NoError(t, err)
}

func TestGrant(t *testing.T) {
	contract := NewContract([]byte{}, fakeAccess{}, fakeStore{})
	err := contract.grant(fakeStore{}, makeStep(t))
	require.EqualError(t, err, "'access:grant_id' not found in tx arg")

	err = contract.grant(fakeStore{}, makeStep(t, GrantIDArg, "x"))
	require.EqualError(t, err, "failed to decode id from tx arg: encoding/hex: invalid byte: U+0078 'x'")

	err = contract.grant(fakeStore{}, makeStep(t, GrantIDArg, "deadbeef"))
	require.EqualError(t, err, "'access:grant_contract' not found in tx arg")

	err = contract.grant(fakeStore{}, makeStep(t, GrantIDArg, "deadbeef",
		GrantContractArg, "fake"))
	require.EqualError(t, err, "'access:grant_command' not found in tx arg")

	err = contract.grant(fakeStore{}, makeStep(t, GrantIDArg, "deadbeef",
		GrantContractArg, "fake contract",
		GrantCommandArg, "fake command"))
	require.EqualError(t, err, "'access:identity' not found in tx arg")

	err = contract.grant(fakeStore{}, makeStep(t, GrantIDArg, "deadbeef",
		GrantContractArg, "fake contract",
		GrantCommandArg, "fake command",
		IdentityArg, "x"))
	require.EqualError(t, err, "failed to decode base64ID: illegal base64 data at input byte 0")

	err = contract.grant(fakeStore{}, makeStep(t, GrantIDArg, "deadbeef",
		GrantContractArg, "fake contract",
		GrantCommandArg, "fake command",
		IdentityArg, "AA=="))
	require.EqualError(t, err, "failed to get public key: bn256.G2: not enough data")

	signer := bls.NewSigner()
	buf, err := signer.GetPublicKey().MarshalBinary()
	require.NoError(t, err)
	id := base64.StdEncoding.EncodeToString(buf)
	err = contract.grant(fakeStore{}, makeStep(t, GrantIDArg, "deadbeef",
		GrantContractArg, "fake contract",
		GrantCommandArg, "fake command",
		IdentityArg, id))
	require.NoError(t, err)

	contract = NewContract([]byte{}, fakeAccess{err: fake.GetError()}, fakeStore{})
	err = contract.grant(fakeStore{}, makeStep(t, GrantIDArg, "deadbeef",
		GrantContractArg, "fake contract",
		GrantCommandArg, "fake command",
		IdentityArg, id))
	require.EqualError(t, err, fake.Err("failed to grant"))
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
