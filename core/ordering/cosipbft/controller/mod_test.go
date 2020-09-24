package controller

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestMinimal_SetCommands(t *testing.T) {
	m := NewMinimal()

	b := node.NewBuilder()
	m.SetCommands(b.(node.Builder))
}

func TestMinimal_Inject(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "dela-test-")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	m := NewMinimal()

	fset := make(node.FlagSet)
	fset["config"] = dir

	inj := node.NewInjector()
	inj.Inject(fake.Mino{})

	err = m.Inject(fset, inj)
	require.NoError(t, err)

	err = m.Inject(fset, node.NewInjector())
	require.EqualError(t, err, "injector: couldn't find dependency for 'mino.Mino'")

	inj.Inject(fake.NewBadMino())
	err = m.Inject(fset, inj)
	require.EqualError(t, err, "pool: failed to listen: couldn't create the rpc: fake error")
}

func TestLoadSigner(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "dela-cosipbft")
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	signer, err := loadSigner(dir)
	require.NoError(t, err)
	require.NotNil(t, signer)
	require.True(t, fileExists(t, filepath.Join(dir, privateKeyFile)))

	if runtime.GOOS != "windows" {
		_, err = loadSigner("/")
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to create file: ")
	}
}

// -----------------------------------------------------------------------------
// Utility functions

func fileExists(t *testing.T, path string) bool {
	stat, err := os.Stat(path)

	return !os.IsNotExist(err) && !stat.IsDir()
}
