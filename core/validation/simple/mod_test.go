package simple

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/internal/testing/fake"
	"golang.org/x/xerrors"
)

func TestService_GetFactory(t *testing.T) {
	srvc := NewService(fakeExec{}, nil)
	require.NotNil(t, srvc.GetFactory())
}

func TestService_GetNonce(t *testing.T) {
	srvc := NewService(fakeExec{}, nil)

	nonce, err := srvc.GetNonce(fakeSnapshot{}, fake.PublicKey{})
	require.NoError(t, err)
	require.Equal(t, uint64(0), nonce)

	buffer := make([]byte, 8)
	buffer[0] = 2
	nonce, err = srvc.GetNonce(fakeSnapshot{value: buffer}, fake.PublicKey{})
	require.NoError(t, err)
	require.Equal(t, uint64(3), nonce)

	_, err = srvc.GetNonce(fakeSnapshot{}, fake.NewBadPublicKey())
	require.EqualError(t, err, fake.Err("key: failed to marshal identity"))

	_, err = srvc.GetNonce(fakeSnapshot{errGet: xerrors.New("oops")}, fake.PublicKey{})
	require.EqualError(t, err, "store: oops")
}

func TestService_Validate(t *testing.T) {
	srvc := NewService(fakeExec{}, nil)

	data, err := srvc.Validate(fakeSnapshot{}, []txn.Transaction{newTx()})
	require.NoError(t, err)
	require.NotNil(t, data)

	tx := newTx()
	tx.nonce = 1
	data, err = srvc.Validate(fakeSnapshot{}, []txn.Transaction{tx})
	require.NoError(t, err)

	status, _ := data.GetTransactionResults()[0].GetStatus()
	require.False(t, status)

	_, err = srvc.Validate(fakeSnapshot{}, []txn.Transaction{fakeTx{}})
	require.EqualError(t, err, "tx 0x0a0b0c0d: nonce: missing identity in transaction")

	_, err = srvc.Validate(fakeSnapshot{errSet: xerrors.New("oops")}, []txn.Transaction{newTx()})
	require.EqualError(t, err, "tx 0x0a0b0c0d: failed to set nonce: store: oops")

	srvc.hashFac = fake.NewHashFactory(fake.NewBadHash())
	err = srvc.set(fakeSnapshot{}, fake.PublicKey{}, 0)
	require.EqualError(t, err, fake.Err("key: failed to write identity"))

	srvc.hashFac = crypto.NewSha256Factory()
	srvc.execution = fakeExec{err: xerrors.New("oops")}
	_, err = srvc.Validate(fakeSnapshot{}, []txn.Transaction{newTx()})
	require.EqualError(t, err, "tx 0x0a0b0c0d: failed to execute tx: oops")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeExec struct {
	err error
}

func (e fakeExec) Execute(txn.Transaction, store.Snapshot) (execution.Result, error) {
	return execution.Result{}, e.err
}

type fakeSnapshot struct {
	store.Snapshot

	value  []byte
	errGet error
	errSet error
}

func (s fakeSnapshot) Get(key []byte) ([]byte, error) {
	return s.value, s.errGet
}

func (s fakeSnapshot) Set(key, value []byte) error {
	return s.errSet
}
