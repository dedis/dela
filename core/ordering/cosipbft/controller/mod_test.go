package controller

import (
	"io/ioutil"
	"os"
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
