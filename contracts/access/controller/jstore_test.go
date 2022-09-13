package controller

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde/json"
)

// constant holding the temporary dela directory name
const delaTestDir = "dela-test-"

func TestJstore_New(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), delaTestDir)
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "store.json")
	store, err := newJstore(path)
	require.NoError(t, err)

	require.NotNil(t, store)

	_, err = newJstore(dir)
	require.Regexp(t, "^failed to read file", err.Error())

	err = os.WriteFile(path, []byte(""), os.ModePerm)
	require.NoError(t, err)

	_, err = newJstore(path)
	require.EqualError(t, err, "failed to read json: unexpected end of JSON input")

	_, err = newJstore("/fake/file")
	require.Regexp(t, "^failed to save empty file:", err.Error())
}

func TestJstore_Set_Get_Delete(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), delaTestDir)
	require.NoError(t, err)

	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "store.json")
	store, err := newJstore(path)
	require.NoError(t, err)

	key := []byte("key")
	val := []byte("value")

	resp, err := store.Get(key)
	require.NoError(t, err)
	require.Nil(t, resp)

	err = store.Set(key, val)
	require.NoError(t, err)

	resp, err = store.Get(key)
	require.NoError(t, err)
	require.Equal(t, val, resp)

	err = store.Delete(key)
	require.NoError(t, err)

	resp, err = store.Get(key)
	require.NoError(t, err)
	require.Nil(t, resp)
}

func TestJstore_SaveFile(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), delaTestDir)
	require.NoError(t, err)

	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "store.json")
	store, err := newJstore(path)
	require.NoError(t, err)

	store.(*jstore).data["key"] = []byte("value")

	err = store.(*jstore).saveFile()
	require.NoError(t, err)

	store.(*jstore).ctx = fake.NewBadContext()
	err = store.(*jstore).saveFile()
	require.EqualError(t, err, fake.Err("failed to marshal data"))

	store.(*jstore).path = dir
	store.(*jstore).ctx = json.NewContext()
	err = store.(*jstore).saveFile()
	require.Regexp(t, "^failed to save file", err.Error())
}

func TestJstore_Scenario(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), delaTestDir)
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "store.json")
	store, err := newJstore(path)
	require.NoError(t, err)

	key1 := []byte("key1")
	val1 := []byte("value1")

	key2 := []byte("key2")
	val2 := []byte("value2")

	err = store.Set(key1, val1)
	require.NoError(t, err)

	err = store.Set(key2, val2)
	require.NoError(t, err)

	// Reading and updating the file with a new store

	store, err = newJstore(path)
	require.NoError(t, err)

	resp, err := store.Get(key1)
	require.NoError(t, err)
	require.Equal(t, val1, resp)

	resp, err = store.Get(key2)
	require.NoError(t, err)
	require.Equal(t, val2, resp)

	err = store.Delete(key1)
	require.NoError(t, err)

	// Reading with a 3rd store to see the update

	store, err = newJstore(path)
	require.NoError(t, err)

	resp, err = store.Get(key1)
	require.NoError(t, err)
	require.Nil(t, resp)

	resp, err = store.Get(key2)
	require.NoError(t, err)
	require.Equal(t, val2, resp)
}
