package controller

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/pool"
	"go.dedis.ch/dela/core/txn/pool/mem"
	"go.dedis.ch/dela/core/txn/signed"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestExecute(t *testing.T) {
	ctx := node.Context{
		Injector: node.NewInjector(),
		Flags:    make(node.FlagSet),
		Out:      io.Discard,
	}

	ctx.Flags.(node.FlagSet)["args"] = []interface{}{"1", "2"}

	action := addAction{client: &client{}}
	ctx.Injector.Inject(mem.NewPool())

	buf, err := bls.NewSigner().MarshalBinary()
	require.NoError(t, err)

	keyFile := filepath.Join(os.TempDir(), "key.buf")
	ctx.Flags.(node.FlagSet)[signerFlag] = keyFile

	err = os.WriteFile(keyFile, buf, os.ModePerm)
	require.NoError(t, err)
	defer os.RemoveAll(keyFile)

	err = action.Execute(ctx)
	require.NoError(t, err)

	ctx.Injector = node.NewInjector()
	ctx.Injector.Inject(&badPool{})
	err = action.Execute(ctx)
	require.EqualError(t, err, "failed to include tx: "+fake.Err("failed to add"))

	getManager = func(c crypto.Signer, s signed.Client) txn.Manager {
		return badManager{}
	}

	err = action.Execute(ctx)
	require.EqualError(t, err, "creating transaction: "+fake.Err("make fail"))

	getManager = func(c crypto.Signer, s signed.Client) txn.Manager {
		return badManager{failSync: true}
	}

	err = action.Execute(ctx)
	require.EqualError(t, err, "failed to sync manager: "+fake.Err("sync fail"))

	err = os.WriteFile(keyFile, []byte("bad signer"), os.ModePerm)
	require.NoError(t, err)

	err = action.Execute(ctx)
	require.EqualError(t, err, "failed to get signer: failed to unmarshal signer: while unmarshaling scalar: UnmarshalBinary: wrong size buffer")

	ctx.Flags.(node.FlagSet)[signerFlag] = "/not/exist"

	err = action.Execute(ctx)
	// the error message can be different based on the platform
	require.Regexp(t, "^failed to get signer: failed to load signer: while opening file: open /not/exist:", err.Error())

	ctx.Flags.(node.FlagSet)["args"] = []interface{}{"1"}

	err = action.Execute(ctx)
	require.EqualError(t, err, "failed to get args: number of args should be even")

	ctx.Injector = node.NewInjector()
	err = action.Execute(ctx)
	require.EqualError(t, err, "injector: couldn't find dependency for 'pool.Pool'")
}

// -----------------------------------------------------------------------------
// Utility functions

type badPool struct {
	pool.Pool
}

func (p *badPool) Add(txn.Transaction) error {
	return errors.New(fake.Err("failed to add"))
}

type badManager struct {
	txn.Manager
	failSync bool
}

func (m badManager) Sync() error {
	if m.failSync {
		return errors.New(fake.Err("sync fail"))
	}

	return nil
}

func (m badManager) Make(args ...txn.Arg) (txn.Transaction, error) {
	return nil, errors.New(fake.Err("make fail"))
}
