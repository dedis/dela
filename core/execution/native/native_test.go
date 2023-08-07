package native

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/execution"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/testing/fake"
)

func TestService_RequireUniqueContractName(t *testing.T) {
	srvc := NewExecution()
	srvc.Set("abc", fakeExec{uid: "abcd"})

	require.PanicsWithError(t, "contract 'abc' already registered", func() {
		srvc.Set("abc", fakeExec{uid: "badd"})
	})
}

func TestService_RequireUniqueContractID(t *testing.T) {
	srvc := NewExecution()
	srvc.Set("abc", fakeExec{uid: "abcd"})

	err := fmt.Sprintf("contract UID '%x' for '%s' already registered",
		"abcd", "bad")

	require.PanicsWithError(t, err, func() {
		srvc.Set("bad", fakeExec{uid: "abcd"})
	})
}

func TestService_VerifyContractIDFormat(t *testing.T) {
	srvc := NewExecution()
	err := fmt.Sprintf("contract UID '%x' for '%s' is not 4 bytes long",
		"abc", "bad")

	require.PanicsWithError(t, err, func() {
		srvc.Set("bad", fakeExec{uid: "abc"})
	})
}

func TestService_Execute(t *testing.T) {
	srvc := NewExecution()
	srvc.Set("abc", fakeExec{uid: "abcd"})
	srvc.Set("bad", fakeExec{uid: "badd", err: fake.GetError()})

	step := execution.Step{}
	step.Current = fakeTx{contract: "abc"}

	res, err := srvc.Execute(nil, step)
	require.NoError(t, err)
	require.Equal(t, execution.Result{Accepted: true}, res)

	step.Current = fakeTx{contract: "bad"}
	res, err = srvc.Execute(nil, step)
	require.NoError(t, err)
	require.Equal(t, execution.Result{Message: fake.GetError().Error()}, res)

	step.Current = fakeTx{contract: "none"}
	_, err = srvc.Execute(nil, step)
	require.EqualError(t, err, "unknown contract 'none'")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeExec struct {
	err error
	uid string
}

func (e fakeExec) Execute(store.Snapshot, execution.Step) error {
	return e.err
}

func (e fakeExec) UID() string {
	return e.uid
}

type fakeTx struct {
	txn.Transaction
	contract string
}

func (tx fakeTx) GetArg(key string) []byte {
	return []byte(tx.contract)
}
