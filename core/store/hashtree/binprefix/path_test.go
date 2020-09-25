package binprefix

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestPath_GetKey(t *testing.T) {
	path := newPath([]byte{}, []byte("ping"))

	require.Equal(t, []byte("ping"), path.GetKey())
}

func TestPath_GetValue(t *testing.T) {
	path := newPath([]byte{}, []byte("ping"))

	require.Nil(t, path.GetValue())

	path.value = []byte("pong")
	require.Equal(t, []byte("pong"), path.GetValue())
}

func TestPath_GetRoot(t *testing.T) {
	path := newPath([]byte{}, []byte("ping"))

	require.Nil(t, path.GetRoot())

	path.root = []byte("pong")
	require.Equal(t, []byte("pong"), path.GetRoot())
}

func TestPath_ComputeRoot(t *testing.T) {
	path := newPath([]byte{1, 2, 3}, []byte("A"))

	root, err := path.computeRoot(fake.NewHashFactory(&fake.Hash{}))
	require.NoError(t, err)
	require.NotEmpty(t, root)

	_, err = path.computeRoot(fake.NewHashFactory(fake.NewBadHash()))
	require.EqualError(t, err, fake.Err("while preparing: empty node failed"))
}
