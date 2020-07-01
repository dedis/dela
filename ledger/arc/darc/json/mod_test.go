package json

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/ledger/arc/darc"
	"go.dedis.ch/dela/serde"
)

func TestAccessFormat_Encode(t *testing.T) {
	access := darc.NewAccess(darc.WithRule("A", []string{"C"}), darc.WithRule("B", nil))

	format := accessFormat{}
	ctx := serde.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, access)
	require.NoError(t, err)
	require.Equal(t, `{"Rules":{"A":["C"],"B":[]}}`, string(data))
}

func TestAccessFormat_Decode(t *testing.T) {
	format := accessFormat{}
	ctx := serde.NewContext(fake.ContextEngine{})

	access, err := format.Decode(ctx, []byte(`{"Rules":{"A":["B"],"C":[]}}`))
	require.NoError(t, err)
	expected := darc.NewAccess(darc.WithRule("A", []string{"B"}), darc.WithRule("C", nil))
	require.Equal(t, expected, access)

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, "couldn't deserialize access: fake error")
}

func TestTaskFormat_Encode(t *testing.T) {
	task := darc.NewServerTask([]byte{1}, darc.NewAccess())

	format := newTaskFormat()
	ctx := serde.NewContext(fake.ContextEngine{})

	data, err := format.Encode(ctx, task)
	require.NoError(t, err)
	require.Equal(t, `{"Key":"AQ==","Access":{"Rules":{}}}`, string(data))

	format.accessFormat = fake.NewBadFormat()
	_, err = format.Encode(ctx, task)
	require.EqualError(t, err, "couldn't serialize access: fake error")
}

func TestTaskFormat_Decode(t *testing.T) {
	format := newTaskFormat()
	ctx := serde.NewContext(fake.ContextEngine{})

	task, err := format.Decode(ctx, []byte(`{"Key":"AQ==","Access":{}}`))
	require.NoError(t, err)
	require.Equal(t, darc.NewServerTask([]byte{1}, darc.NewAccess()), task)

	_, err = format.Decode(fake.NewBadContext(), []byte(`{}`))
	require.EqualError(t, err, "couldn't deserialize task: fake error")

	format.accessFormat = fake.NewBadFormat()
	_, err = format.Decode(ctx, []byte(`{"Key":"AQ==","Access":{}}`))
	require.EqualError(t, err, "couldn't deserialize access: fake error")
}
