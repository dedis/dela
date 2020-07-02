package json

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/ledger/byzcoin/memship"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func TestTaskFormat_Encode(t *testing.T) {
	task := memship.NewServerTask(fakeAuthority{})

	format := taskFormat{}
	ctx := serde.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, task)
	require.NoError(t, err)
	require.Regexp(t, `{"Authority":{}}`, string(data))

	_, err = format.Encode(ctx, task.ClientTask)
	require.NoError(t, err)

	_, err = format.Encode(ctx, fake.Message{})
	require.EqualError(t, err, "unsupported message of type 'fake.Message'")

	_, err = format.Encode(fake.NewBadContext(), task)
	require.EqualError(t, err, "couldn't marshal: fake error")

	task = memship.NewServerTask(fakeAuthority{err: xerrors.New("oops")})
	_, err = format.Encode(ctx, task)
	require.EqualError(t, err, "couldn't serialize authority: oops")
}

func TestTaskFormat_Decode(t *testing.T) {
	format := taskFormat{}
	ctx := serde.NewContext(fake.ContextEngine{})
	ctx = serde.WithFactory(ctx, memship.RosterKey{}, fakeAuthorityFactory{})

	task, err := format.Decode(ctx, []byte(`{"Authority":[{}]}`))
	require.NoError(t, err)
	require.Equal(t, memship.NewServerTask(fakeAuthority{}), task)

	_, err = format.Decode(fake.NewBadContext(), []byte(`{"Authority":[{}]}`))
	require.EqualError(t, err, "couldn't deserialize task: fake error")

	badCtx := serde.WithFactory(ctx, memship.RosterKey{}, fakeAuthorityFactory{err: xerrors.New("oops")})
	_, err = format.Decode(badCtx, []byte(`{"Authority":[{}]}`))
	require.EqualError(t, err, "couldn't deserialize roster: oops")

	badCtx = serde.WithFactory(ctx, memship.RosterKey{}, nil)
	_, err = format.Decode(badCtx, []byte(`{}`))
	require.EqualError(t, err, "invalid factory of type '<nil>'")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeAuthority struct {
	viewchange.Authority

	err error
}

func (f fakeAuthority) Serialize(serde.Context) ([]byte, error) {
	return []byte(`{}`), f.err
}

type fakeAuthorityFactory struct {
	viewchange.AuthorityFactory

	err error
}

func (f fakeAuthorityFactory) AuthorityOf(serde.Context, []byte) (viewchange.Authority, error) {
	return fakeAuthority{}, f.err
}
