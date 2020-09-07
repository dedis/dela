package simple

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/anon"
	"go.dedis.ch/dela/crypto/bls"
	"golang.org/x/xerrors"
)

func TestService_GetFactory(t *testing.T) {
	srvc := NewService(fakeExec{}, nil)
	require.NotNil(t, srvc.GetFactory())
}

func TestService_Validate(t *testing.T) {
	srvc := NewService(fakeExec{}, nil)

	data, err := srvc.Validate(fakeSnapshot{}, []txn.Transaction{makeTx(t)})
	require.NoError(t, err)
	require.NotNil(t, data)

	srvc.execution = fakeExec{err: xerrors.New("oops")}
	_, err = srvc.Validate(fakeSnapshot{}, []txn.Transaction{makeTx(t)})
	require.EqualError(t, err, "failed to execute tx: oops")
}

// -----------------------------------------------------------------------------
// Utility functions

func makeTx(t *testing.T) txn.Transaction {
	signer := bls.NewSigner()

	tx, err := anon.NewTransaction(0, signer.GetPublicKey())
	require.NoError(t, err)
	return tx
}

type fakeExec struct {
	err error
}

func (e fakeExec) Execute(txn.Transaction, store.Snapshot) (execution.Result, error) {
	return execution.Result{}, e.err
}

type fakeSnapshot struct {
	store.Snapshot
}

func (fakeSnapshot) Get(key []byte) ([]byte, error) {
	return nil, nil
}

func (fakeSnapshot) Set(key, value []byte) error {
	return nil
}
