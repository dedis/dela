package mem

import (
	"testing"

	"github.com/stretchr/testify/require"
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
