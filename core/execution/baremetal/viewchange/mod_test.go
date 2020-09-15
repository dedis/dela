package viewchange

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/access"
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/signed"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func TestNewTransaction(t *testing.T) {
	mgr := NewManager(signed.NewManager(fake.NewSigner(), nil))

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

	contract := NewContract([]byte("roster"), []byte("access"), fac, fakeAccessService{})

	err := contract.Execute(makeTx(t, "[]"), fakeStore{})
	require.NoError(t, err)

	contract.rosterFac = badRosterFac{}
	err = contract.Execute(makeTx(t, "[]"), fakeStore{})
	require.EqualError(t, err, messageArgMissing)

	contract.rosterFac = fac
	err = contract.Execute(makeTx(t, "[]"), fakeStore{errGet: xerrors.New("oops")})
	require.EqualError(t, err, messageStorageEmpty)

	contract.rosterFac = badRosterFac{counter: fake.NewCounter(1)}
	err = contract.Execute(makeTx(t, "[]"), fakeStore{})
	require.EqualError(t, err, messageStorageCorrupted)

	contract.rosterFac = fac
	err = contract.Execute(makeTx(t, "[{},{},{}]"), fakeStore{})
	require.EqualError(t, err, messageTooManyChanges)

	err = contract.Execute(makeTx(t, "[{},{}]"), fakeStore{})
	require.EqualError(t, err, "duplicate in roster: fake.Address[0]")

	err = contract.Execute(makeTx(t, "[]"), fakeStore{errSet: xerrors.New("oops")})
	require.EqualError(t, err, messageStorageFailure)
}

// -----------------------------------------------------------------------------
// Utility functions

func makeTx(t *testing.T, arg string) txn.Transaction {
	tx, err := signed.NewTransaction(0, fake.PublicKey{}, signed.WithArg(AuthorityArg, []byte(arg)))
	require.NoError(t, err)

	return tx
}

type fakeStore struct {
	store.Snapshot

	errGet error
	errSet error
}

func (snap fakeStore) Get(key []byte) ([]byte, error) {
	return []byte("[{}]"), snap.errGet
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

type fakeAccessService struct {
	access.Service
}

func (fakeAccessService) Match(store.Readable, access.Credentials, ...access.Identity) error {
	return nil
}
