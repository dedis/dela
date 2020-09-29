package simple

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/validation"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/internal/testing/fake"
	"golang.org/x/xerrors"
)

func TestService_GetFactory(t *testing.T) {
	srvc := NewService(&fakeExec{}, nil)
	require.NotNil(t, srvc.GetFactory())
}

func TestService_GetNonce(t *testing.T) {
	srvc := NewService(&fakeExec{}, nil)

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

	_, err = srvc.GetNonce(fakeSnapshot{errGet: fake.GetError()}, fake.PublicKey{})
	require.EqualError(t, err, fake.Err("store"))
}

func TestService_Accept(t *testing.T) {
	srvc := NewService(fakeExec{}, nil)

	tx := newTx()
	tx.nonce = 5

	err := srvc.Accept(fakeSnapshot{}, tx, validation.Leeway{MaxSequenceDifference: 5})
	require.NoError(t, err)

	err = srvc.Accept(fakeSnapshot{}, fakeTx{}, validation.Leeway{})
	require.EqualError(t, err, "while reading nonce: missing identity in transaction")

	value := make([]byte, 8)
	value[0] = 5
	err = srvc.Accept(fakeSnapshot{value: value}, newTx(), validation.Leeway{})
	require.EqualError(t, err, "nonce '0' < '6'")

	err = srvc.Accept(fakeSnapshot{}, tx, validation.Leeway{MaxSequenceDifference: 1})
	require.EqualError(t, err, "nonce '5' above the limit '1'")
}

func TestService_Validate(t *testing.T) {
	exec := &fakeExec{check: true}
	srvc := NewService(exec, nil)

	data, err := srvc.Validate(fakeSnapshot{}, []txn.Transaction{newTx(), newTx(), newTx()})
	require.NoError(t, err)
	require.NotNil(t, data)
	require.Equal(t, 3, exec.count)

	tx := newTx()
	tx.nonce = 1
	data, err = srvc.Validate(fakeSnapshot{}, []txn.Transaction{tx})
	require.NoError(t, err)

	status, _ := data.GetTransactionResults()[0].GetStatus()
	require.False(t, status)

	srvc.execution = &fakeExec{}
	_, err = srvc.Validate(fakeSnapshot{}, []txn.Transaction{fakeTx{}})
	require.EqualError(t, err, "tx 0x0a0b0c0d: nonce: missing identity in transaction")

	_, err = srvc.Validate(fakeSnapshot{errSet: fake.GetError()}, []txn.Transaction{newTx()})
	require.EqualError(t, err, fake.Err("tx 0x0a0b0c0d: failed to set nonce: store"))

	srvc.hashFac = fake.NewHashFactory(fake.NewBadHash())
	err = srvc.set(fakeSnapshot{}, fake.PublicKey{}, 0)
	require.EqualError(t, err, fake.Err("key: failed to write identity"))

	srvc.hashFac = crypto.NewSha256Factory()
	srvc.execution = &fakeExec{err: fake.GetError()}
	_, err = srvc.Validate(fakeSnapshot{}, []txn.Transaction{newTx()})
	require.EqualError(t, err, fake.Err("tx 0x0a0b0c0d: failed to execute tx"))
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeExec struct {
	err   error
	count int
	check bool
}

func (e *fakeExec) Execute(store store.Snapshot, step execution.Step) (execution.Result, error) {
	if e.check && e.count != len(step.Previous) {
		return execution.Result{}, xerrors.New("missing previous txs")
	}

	e.count++
	return execution.Result{Accepted: true}, e.err
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
