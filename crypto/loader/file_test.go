package loader

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestFileLoader_LoadOrCreate(t *testing.T) {
	file, err := os.CreateTemp(os.TempDir(), "dela")
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
}

func TestFileLoader_BadGenerator_LoadOrCreate(t *testing.T) {
	loader := fileLoader{
		path:   "",
		statFn: statNotExists,
	}

	_, err := loader.LoadOrCreate(fakeGenerator{err: fake.GetError()})
	require.EqualError(t, err, fake.Err("generator failed"))
}

func TestFileLoader_FailCreateFile_LoadOrCreate(t *testing.T) {
	loader := fileLoader{
		path:   "",
		statFn: statNotExists,
		openFileFn: func(path string, flags int, perms os.FileMode) (*os.File, error) {
			return nil, fake.GetError()
		},
	}

	_, err := loader.LoadOrCreate(fakeGenerator{})
	require.EqualError(t, err, fake.Err("while creating file"))
}

func TestFileLoader_FailWriteFile_LoadOrCreate(t *testing.T) {
	loader := fileLoader{
		path:   "",
		statFn: statNotExists,
		openFileFn: func(path string, flags int, perms os.FileMode) (*os.File, error) {
			return os.NewFile(0, ""), nil
		},
	}

	_, err := loader.LoadOrCreate(fakeGenerator{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "while writing: write : ")
}

func TestFileLoader_FailOpenFile_LoadOrCreate(t *testing.T) {
	loader := fileLoader{
		path:   "",
		statFn: statExists,
		openFn: func(path string) (*os.File, error) {
			return nil, fake.GetError()
		},
	}

	_, err := loader.LoadOrCreate(fakeGenerator{})
	require.EqualError(t, err, fake.Err("failed to load file: while opening file"))
}

func TestFileLoader_FailReadFile_LoadOrCreate(t *testing.T) {
	loader := fileLoader{
		path:   "",
		statFn: statExists,
		openFn: func(path string) (*os.File, error) {
			return os.Open(os.TempDir())
		},
	}

	_, err := loader.LoadOrCreate(fakeGenerator{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "while reading file: ")
}

// -----------------------------------------------------------------------------
// Utility functions

func statNotExists(path string) (os.FileInfo, error) {
	return nil, os.ErrNotExist
}

func statExists(path string) (os.FileInfo, error) {
	return nil, nil
}

type fakeGenerator struct {
	calls *fake.Call
	err   error
}

func (g fakeGenerator) Generate() ([]byte, error) {
	g.calls.Add("Generate")

	return []byte{1, 2, 3}, g.err
}
