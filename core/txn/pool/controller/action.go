// This file implements the actions of the controller
//
// Documentation Last Review: 02.02.2021
//

package controller

import (
	"sync"

	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/crypto/loader"

	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/core/txn"
	"go.dedis.ch/dela/core/txn/pool"
	"go.dedis.ch/dela/core/txn/signed"
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
	sync.Mutex

	client *client
}

// Execute implements node.ActionTemplate
func (a *addAction) Execute(ctx node.Context) error {
	a.Lock()
	defer a.Unlock()

	var p pool.Pool
	err := ctx.Injector.Resolve(&p)
	if err != nil {
		return xerrors.Errorf("injector: %v", err)
	}

	args, err := getArgs(ctx)
	if err != nil {
		return xerrors.Errorf("failed to get args: %v", err)
	}

	signer, err := getSigner(ctx)
	if err != nil {
		return xerrors.Errorf("failed to get signer: %v", err)
	}

	nonce := ctx.Flags.Int(nonceFlag)
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

// getArgs extracts and parses arguments from the context.
func getArgs(ctx node.Context) ([]txn.Arg, error) {
	inArgs := ctx.Flags.StringSlice("args")
	if len(inArgs)%2 != 0 {
		return nil, xerrors.New("number of args should be even")
	}

	args := make([]txn.Arg, len(inArgs)/2)
	for i := 0; i < len(args); i++ {
		args[i] = txn.Arg{
			Key:   inArgs[i*2],
			Value: []byte(inArgs[i*2+1]),
		}
	}

	return args, nil
}

// getSigner creates a signer from the signerFlag flag in context.
func getSigner(ctx node.Context) (crypto.Signer, error) {
	l := loader.NewFileLoader(ctx.Flags.Path(signerFlag))

	signerdata, err := l.Load()
	if err != nil {
		return nil, xerrors.Errorf("failed to load signer: %v", err)
	}

	signer, err := bls.NewSignerFromBytes(signerdata)
	if err != nil {
		return nil, xerrors.Errorf("failed to unmarshal signer: %v", err)
	}

	return signer, nil
}
