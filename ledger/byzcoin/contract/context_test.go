package contract

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func TestTaskContext_GetArc(t *testing.T) {
	ctx := taskContext{
		page: fakePage{
			store: map[string]serde.Message{"a": &fakeAccess{}},
		},
	}

	arc, err := ctx.GetArc([]byte("a"))
	require.NoError(t, err)
	require.NotNil(t, arc)

	ctx.page = fakePage{errRead: xerrors.New("oops")}
	_, err = ctx.GetArc(nil)
	require.EqualError(t, err, "couldn't read from page: oops")
}

func TestTaskContext_Read(t *testing.T) {
	ctx := taskContext{
		page: fakePage{
			store: map[string]serde.Message{
				"a": &Instance{
					ContractID: "abc",
					Value:      fake.Message{},
				},
				"b": fake.Message{},
			},
		},
	}

	instance, err := ctx.Read([]byte("a"))
	require.NoError(t, err)
	require.Equal(t, "abc", instance.ContractID)
	require.NotNil(t, instance.Value)

	_, err = ctx.Read(nil)
	require.EqualError(t, err,
		"invalid message type '<nil>' != '*contract.Instance'")

	_, err = ctx.Read([]byte("b"))
	require.EqualError(t, err,
		"invalid message type 'fake.Message' != '*contract.Instance'")

	ctx.page = fakePage{errRead: xerrors.New("oops")}
	_, err = ctx.Read(nil)
	require.EqualError(t, err, "couldn't read from page: oops")
}
