package controller

import (
	"encoding/base64"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cli"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/core/execution/native"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestAddAction_Execute(t *testing.T) {
	ctx := node.Context{
		Injector: node.NewInjector(),
		Flags:    make(node.FlagSet),
		Out:      io.Discard,
	}

	action := addAction{}
	err := action.Execute(ctx)
	require.EqualError(t, err, "failed to resolve native service: couldn't find dependency for '*native.Service'")

	native := native.NewExecution()
	ctx.Injector.Inject(native)

	err = action.Execute(ctx)
	require.EqualError(t, err, "failed to resolve access service: couldn't find dependency for 'access.Service'")

	access := fakeAccess{}
	ctx.Injector.Inject(&access)

	err = action.Execute(ctx)
	require.EqualError(t, err, "failed to resolve access store: couldn't find dependency for 'controller.accessStore'")

	store := fakeStore{}
	ctx.Injector.Inject(&store)

	err = action.Execute(ctx)
	require.NoError(t, err)

	access.err = fake.GetError()

	err = action.Execute(ctx)
	require.EqualError(t, err, fake.Err("failed to grant"))

	flags := fakeFlags{strings: make(map[string][]string)}
	ctx.Flags = flags
	flags.strings["identity"] = []string{"a"}

	err = action.Execute(ctx)
	require.EqualError(t, err, "failed to parse identities: failed to decode pub key 'a': illegal base64 data at input byte 0")

	flags.strings["identity"] = []string{"AA=="}

	err = action.Execute(ctx)
	require.EqualError(t, err, "failed to parse identities: failed to unmarshal identity 'AA==': bn256.G2: not enough data")

	signer := bls.NewSigner()
	buf, err := signer.GetPublicKey().MarshalBinary()
	require.NoError(t, err)
	id := base64.StdEncoding.EncodeToString(buf)
	flags.strings["identity"] = []string{id}

	access.err = nil

	err = action.Execute(ctx)
	require.NoError(t, err)
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeStore struct {
	accessStore
}

type fakeFlags struct {
	cli.Flags

	strings map[string][]string
}

func (f fakeFlags) StringSlice(name string) []string {
	return f.strings[name]
}
