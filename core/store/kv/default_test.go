package kv

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
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

	err = db.View([]byte{0xaa}, nil)
	require.EqualError(t, err, "bucket 'aa' not found")

	err = db.Update(nil, nil)
	require.EqualError(t, err, "failed to create bucket: bucket name required")
}

func TestBoltBucket_Get_Set_Delete(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "dela-core-kv")
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	db, err := New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)

	err = db.Update([]byte("bucket"), func(b Bucket) error {
		require.NoError(t, b.Set([]byte("ping"), []byte("pong")))

		value := b.Get([]byte("ping"))
		require.Equal(t, []byte("pong"), value)

		value = b.Get([]byte("pong"))
		require.Nil(t, value)

		require.NoError(t, b.Delete([]byte("ping")))

		value = b.Get([]byte("ping"))
		require.Nil(t, value)

		return nil
	})

	require.NoError(t, err)
}

func TestBoltBucket_ForEach(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "dela-core-kv")
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	db, err := New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)

	err = db.Update([]byte("bucket"), func(b Bucket) error {
		require.NoError(t, b.Set([]byte{2}, []byte{2}))
		require.NoError(t, b.Set([]byte{1}, []byte{1}))
		require.NoError(t, b.Set([]byte{0}, []byte{0}))

		var i byte = 0
		return b.ForEach(func(k, v []byte) error {
			require.Equal(t, []byte{i}, k)
			require.Equal(t, []byte{i}, v)
			i++
			return nil
		})
	})
	require.NoError(t, err)
}

func TestBoltBucket_Scan(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "dela-core-kv")
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	db, err := New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)

	err = db.Update([]byte("bucket"), func(b Bucket) error {
		require.NoError(t, b.Set([]byte{7}, []byte{7}))
		require.NoError(t, b.Set([]byte{0}, []byte{0}))

		var i byte = 0
		b.Scan(nil, func(k, v []byte) error {
			require.Equal(t, []byte{i}, k)
			require.Equal(t, []byte{i}, v)
			i += 7
			return nil
		})
		require.Equal(t, byte(14), i)

		err = b.Scan([]byte{1}, func(k, v []byte) error {
			return xerrors.New("oops")
		})
		require.NoError(t, err)

		err = b.Scan([]byte{}, func(k, v []byte) error {
			return xerrors.New("oops")
		})
		require.EqualError(t, err, "callback failed: oops")

		return nil
	})
	require.NoError(t, err)
}
