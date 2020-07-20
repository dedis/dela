package kv

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBoltDB_UpdateAndView(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "dela-core-kv")
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	db, err := New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)

	err = db.Update([]byte("bucket"), func(b Bucket) error {
		return b.Set([]byte("ping"), []byte("pong"))
	})
	require.NoError(t, err)

	err = db.View([]byte("bucket"), func(b Bucket) error {
		value := b.Get([]byte("ping"))
		require.Equal(t, []byte("pong"), value)

		return nil
	})
	require.NoError(t, err)
}
