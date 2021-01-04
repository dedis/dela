package controller

import (
	"go.dedis.ch/dela/crypto"

	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/pool"
	"go.dedis.ch/dela/core/txn/signed"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/crypto/loader"
	"golang.org/x/xerrors"
)

// getManager is the function called when we need a transaction manager. It
// allows us to use a different manager for the tests.
var getManager = func(signer crypto.Signer, s signed.Client) txn.Manager {
	return signed.NewManager(signer, s)
}

// addAction describes an action to add an new transaction to the pool.
//
// - implements node.ActionTemplate
type addAction struct {
	client *client
}

// Execute implements node.ActionTemplate
func (a addAction) Execute(ctx node.Context) error {
	var p pool.Pool
	err := ctx.Injector.Resolve(&p)
	if err != nil {
		return xerrors.Errorf("injector: %v", err)
	}

	inArgs := ctx.Flags.StringSlice("args")
	if len(inArgs)%2 != 0 {
		return xerrors.New("number of args should be even")
	}

	args := make([]txn.Arg, len(inArgs)/2)
	for i := 0; i < len(args); i++ {
		args[i] = txn.Arg{
			Key:   inArgs[i*2],
			Value: []byte(inArgs[i*2+1]),
		}
	}

	l := loader.NewFileLoader(ctx.Flags.Path("key"))

	signerdata, err := l.Load()
	if err != nil {
		return xerrors.Errorf("failed to load signer: %v", err)
	}

	signer, err := bls.NewSignerFromBytes(signerdata)
	if err != nil {
		return xerrors.Errorf("failed to unmarshal signer: %v", err)
	}

	nonce := ctx.Flags.Int("nonce")
	if nonce != -1 {
		a.client.nonce = uint64(nonce)
	}

	manager := getManager(signer, a.client)

	err = manager.Sync()
	if err != nil {
		return xerrors.Errorf("failed to sync manager: %v", err)
	}

	tx, err := manager.Make(args...)
	if err != nil {
		return xerrors.Errorf("creating transaction: %v", err)
	}

	err = p.Add(tx)
	if err != nil {
		return xerrors.Errorf("failed to include tx: %v", err)
	}

	return nil
}
