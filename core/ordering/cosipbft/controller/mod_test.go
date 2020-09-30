package controller

import (
	"encoding"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/core/txn/pool"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestMinimal_SetCommands(t *testing.T) {
	m := NewMinimal()

	b := node.NewBuilder()
	m.SetCommands(b.(node.Builder))
}

func TestMinimal_OnStart(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "dela-test-")
	require.NoError(t, err)

	db, err := kv.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	m := NewMinimal().(minimal)

	fset := make(node.FlagSet)
	fset["config"] = dir

	inj := node.NewInjector()
	inj.Inject(fake.Mino{})
	inj.Inject(db)

	err = m.OnStart(fset, inj)
	require.NoError(t, err)

	err = m.OnStart(fset, node.NewInjector())
	require.EqualError(t, err,
		"injector: couldn't find dependency for 'mino.Mino'")

	require.NoError(t, os.Remove(filepath.Join(dir, privateKeyFile)))
	m.signerFn = badFn
	err = m.OnStart(fset, inj)
	require.EqualError(t, err,
		fake.Err("signer: while loading: generator failed: failed to marshal signer"))

	m.signerFn = blsSigner
	inj.Inject(fake.NewBadMino())
	err = m.OnStart(fset, inj)
	require.EqualError(t, err,
		fake.Err("pool: failed to listen: couldn't create the rpc"))

	inj = node.NewInjector()
	inj.Inject(fake.Mino{})
	err = m.OnStart(fset, inj)
	require.EqualError(t, err, "injector: couldn't find dependency for 'kv.DB'")

	// Test with an invalid private key in the file.
	os.Remove(filepath.Join(dir, privateKeyFile))

	file, err := os.Create(filepath.Join(dir, privateKeyFile))
	require.NoError(t, err)

	file.Close()

	inj.Inject(db)
	err = m.OnStart(fset, inj)
	require.Error(t, err)
	require.Contains(t, err.Error(), "signer: while unmarshaling: ")
}

func TestMinimal_OnStop(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "dela-test-")
	require.NoError(t, err)

	db, err := kv.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)

	defer os.RemoveAll(dir)

	m := NewMinimal()

	fset := make(node.FlagSet)
	fset["config"] = dir

	inj := node.NewInjector()
	inj.Inject(fake.Mino{})
	inj.Inject(db)

	err = m.OnStart(fset, inj)
	require.NoError(t, err)

	err = m.OnStop(inj)
	require.NoError(t, err)

	inj = node.NewInjector()
	err = m.OnStop(inj)
	require.EqualError(t, err,
		"injector: couldn't find dependency for 'ordering.Service'")

	inj.Inject(fakeService{err: fake.GetError()})
	err = m.OnStop(inj)
	require.EqualError(t, err, fake.Err("while closing service"))

	inj.Inject(fakeService{})
	err = m.OnStop(inj)
	require.EqualError(t, err,
		"injector: couldn't find dependency for 'pool.Pool'")

	inj.Inject(fakePool{err: fake.GetError()})
	err = m.OnStop(inj)
	require.EqualError(t, err, fake.Err("while closing pool"))
}

// -----------------------------------------------------------------------------
// Utility functions

func badFn() encoding.BinaryMarshaler {
	return fake.NewBadHash()
}

type fakePool struct {
	pool.Pool

	err error
}

func (p fakePool) Close() error {
	return p.err
}
