package mem

import (
	"bytes"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
)

const testKeyLen = 16

func TestTree_Basic(t *testing.T) {
	tree := NewTree(testKeyLen)
	keys := make([][testKeyLen]byte, 0)

	f := func(key [testKeyLen]byte, value []byte) bool {
		tree.Insert(key[:], value)

		keys = append(keys, key)

		ret, err := tree.Search(key[:], nil)
		require.NoError(t, err)

		return bytes.Equal(value, ret)
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)
	require.Greater(t, tree.Len(), 0)

	clone := tree.Clone()
	require.Equal(t, clone.Len(), tree.Len())

	err = tree.Update()
	require.NoError(t, err)

	for _, key := range keys {
		err = tree.Delete(key[:])
		require.NoError(t, err)
	}

	require.Equal(t, 0, tree.Len())
}
