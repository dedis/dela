package viewchange

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/consensus/viewchange/roster"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/anon"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func TestContract_Execute(t *testing.T) {
	fac := roster.NewFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})

	contract := NewContract([]byte("abc"), fac)

	res, err := contract.Execute(makeTx(t, "[]"), fakeStore{})
	require.NoError(t, err)
	require.True(t, res.Accepted)

	contract.rosterFac = badRosterFac{}
	res, err = contract.Execute(makeTx(t, "[]"), fakeStore{})
	require.EqualError(t, err, "failed to decode arg: oops")
	require.False(t, res.Accepted)
	require.Equal(t, messageArgMissing, res.Message)

	contract.rosterFac = fac
	res, err = contract.Execute(makeTx(t, "[]"), fakeStore{errGet: xerrors.New("oops")})
	require.EqualError(t, err, "failed to read roster: oops")
	require.False(t, res.Accepted)
	require.Equal(t, messageStorageEmpty, res.Message)

	contract.rosterFac = badRosterFac{counter: fake.NewCounter(1)}
	res, err = contract.Execute(makeTx(t, "[]"), fakeStore{})
	require.EqualError(t, err, "failed to decode roster: oops")
	require.False(t, res.Accepted)
	require.Equal(t, messageStorageCorrupted, res.Message)

	contract.rosterFac = fac
	res, err = contract.Execute(makeTx(t, "[{},{}]"), fakeStore{})
	require.EqualError(t, err, "only one change is expected but found 2")
	require.False(t, res.Accepted)
	require.Equal(t, messageTooManyChanges, res.Message)

	res, err = contract.Execute(makeTx(t, "[]"), fakeStore{errSet: xerrors.New("oops")})
	require.EqualError(t, err, "failed to store roster: oops")
	require.False(t, res.Accepted)
	require.Equal(t, messageStorageFailure, res.Message)
}

// Utility functions -----------------------------------------------------------

func makeTx(t *testing.T, arg string) txn.Transaction {
	tx, err := anon.NewTransaction(0, anon.WithArg(AuthorityArg, []byte(arg)))
	require.NoError(t, err)

	return tx
}

type fakeStore struct {
	store.Snapshot

	errGet error
	errSet error
}

func (snap fakeStore) Get(key []byte) ([]byte, error) {
	return []byte("[]"), snap.errGet
}

func (snap fakeStore) Set(key, value []byte) error {
	return snap.errSet
}

type badRosterFac struct {
	viewchange.AuthorityFactory
	counter *fake.Counter
}

func (fac badRosterFac) AuthorityOf(serde.Context, []byte) (viewchange.Authority, error) {
	if fac.counter.Done() {
		return nil, xerrors.New("oops")
	}

	fac.counter.Decrease()
	return nil, nil
}
