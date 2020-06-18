package json

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/ledger/byzcoin/memship"
	"go.dedis.ch/dela/serdeng"
	"golang.org/x/xerrors"
)

func TestTask_VisitJSON(t *testing.T) {
	task := memship.NewServerTask(fakeAuthority{})

	format := taskFormat{}
	ctx := serdeng.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, task)
	require.NoError(t, err)
	require.Regexp(t, `{"Authority":{}}`, string(data))

	task = memship.NewServerTask(fakeAuthority{err: xerrors.New("oops")})
	_, err = format.Encode(ctx, task)
	require.EqualError(t, err, "couldn't serialize authority: oops")
}

func TestTaskManager_VisitJSON(t *testing.T) {
	format := taskFormat{}
	ctx := serdeng.NewContext(fake.ContextEngine{})
	ctx = serdeng.WithFactory(ctx, memship.RosterKey{}, fakeAuthorityFactory{})

	task, err := format.Decode(ctx, []byte(`{"Authority":[{}]}`))
	require.NoError(t, err)
	require.Equal(t, memship.NewServerTask(fakeAuthority{}), task)

	_, err = format.Decode(fake.NewBadContext(), []byte(`{"Authority":[{}]}`))
	require.EqualError(t, err, "couldn't deserialize task: fake error")

	badCtx := serdeng.WithFactory(ctx, memship.RosterKey{}, fakeAuthorityFactory{err: xerrors.New("oops")})
	_, err = format.Decode(badCtx, []byte(`{"Authority":[{}]}`))
	require.EqualError(t, err, "couldn't deserialize roster: oops")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeAuthority struct {
	viewchange.Authority

	err error
}

func (f fakeAuthority) Serialize(serdeng.Context) ([]byte, error) {
	return []byte(`{}`), f.err
}

type fakeAuthorityFactory struct {
	viewchange.AuthorityFactory

	err error
}

func (f fakeAuthorityFactory) AuthorityOf(serdeng.Context, []byte) (viewchange.Authority, error) {
	return fakeAuthority{}, f.err
}
