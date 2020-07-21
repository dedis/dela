package mem

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/store/kv"
)

func TestDiskNode_Search(t *testing.T) {
	const keyLen = 4

	tree, clean := makeTree(t)
	defer clean()

	values := map[[keyLen]byte][]byte{}

	// It stores everything to the disk except the root node.
	tree.memDepth = 0

	f := func(key [keyLen]byte, value []byte) bool {
		err := tree.Insert(key[:], value[:])
		require.NoError(t, err)

		values[key] = value

		return true
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)

	err = tree.Persist()
	require.NoError(t, err)

	for key, expected := range values {
		value, err := tree.Search(key[:], nil)
		require.NoError(t, err)
		require.Equal(t, expected, value)
	}
}

// Utility functions -----------------------------------------------------------

func makeTree(t *testing.T) (*Tree, func()) {
	dir, err := ioutil.TempDir(os.TempDir(), "dela-pow")
	require.NoError(t, err)

	db, err := kv.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)

	tree := NewTree(Nonce{}, db)
	tree.Persist()

	return tree, func() { os.RemoveAll(dir) }
}
