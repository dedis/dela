package loader

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestFileLoader_LoadOrCreate(t *testing.T) {
	file, err := ioutil.TempFile(os.TempDir(), "dela")
	require.NoError(t, err)

	file.Close()

	defer os.Remove(file.Name())
	require.NoError(t, os.Remove(file.Name()))

	generator := fakeGenerator{
		calls: fake.NewCall(),
	}

	loader := NewFileLoader(file.Name()).(fileLoader)

	// Generate..
	data, err := loader.LoadOrCreate(generator)
	require.NoError(t, err)
	require.Equal(t, []byte{1, 2, 3}, data)
	require.Equal(t, 1, generator.calls.Len())

	// Read from the file..
	data, err = loader.LoadOrCreate(generator)
	require.NoError(t, err)
	require.Equal(t, []byte{1, 2, 3}, data)
	require.Equal(t, 1, generator.calls.Len())

	os.Remove(file.Name())
	_, err = loader.LoadOrCreate(fakeGenerator{err: fake.GetError()})
	require.EqualError(t, err, fake.Err("generator failed"))

	loader.statFn = func(path string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}
	loader.openFileFn = func(path string, flags int, perms os.FileMode) (*os.File, error) {
		return nil, fake.GetError()
	}
	_, err = loader.LoadOrCreate(generator)
	require.EqualError(t, err, fake.Err("while creating file"))

	loader.openFileFn = func(path string, flags int, perms os.FileMode) (*os.File, error) {
		return os.NewFile(0, ""), nil
	}
	_, err = loader.LoadOrCreate(generator)
	require.Error(t, err)
	require.Contains(t, err.Error(), "while writing: write : ")

	loader.statFn = func(path string) (os.FileInfo, error) {
		return nil, nil
	}
	loader.openFn = func(path string) (*os.File, error) {
		return nil, fake.GetError()
	}
	_, err = loader.LoadOrCreate(generator)
	require.EqualError(t, err, fake.Err("while opening file"))

	loader.openFn = func(path string) (*os.File, error) {
		return os.Open(os.TempDir())
	}
	_, err = loader.LoadOrCreate(generator)
	require.Error(t, err)
	require.Contains(t, err.Error(), "while reading file: ")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeGenerator struct {
	calls *fake.Call
	err   error
}

func (g fakeGenerator) Generate() ([]byte, error) {
	g.calls.Add("Generate")

	return []byte{1, 2, 3}, g.err
}
