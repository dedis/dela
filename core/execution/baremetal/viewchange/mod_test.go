package viewchange

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/anon"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func TestNewTransaction(t *testing.T) {
	mgr := NewManager(anon.NewManager())

	tx, err := mgr.Make(authority.New(nil, nil))
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, "[]", string(tx.GetArg(AuthorityArg)))

	_, err = mgr.Make(badRoster{})
	require.EqualError(t, err, "failed to serialize roster: oops")

	mgr.manager = badManager{}
	_, err = mgr.Make(authority.New(nil, nil))
	require.EqualError(t, err, "creating transaction: oops")
}

func TestContract_Execute(t *testing.T) {
	fac := authority.NewFactory(fake.AddressFactory{}, fake.PublicKeyFactory{})

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

// -----------------------------------------------------------------------------
// Utility functions

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
	authority.Factory
	counter *fake.Counter
}

func (fac badRosterFac) AuthorityOf(serde.Context, []byte) (authority.Authority, error) {
	if fac.counter.Done() {
		return nil, xerrors.New("oops")
	}

	fac.counter.Decrease()
	return nil, nil
}

type badRoster struct {
	authority.Authority
}

func (ro badRoster) Serialize(serde.Context) ([]byte, error) {
	return nil, xerrors.New("oops")
}

type badManager struct {
	txn.Manager
}

func (badManager) Make(opts ...txn.Arg) (txn.Transaction, error) {
	return nil, xerrors.New("oops")
}
