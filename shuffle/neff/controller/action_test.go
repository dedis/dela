package controller

import (
	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cli/node"
	"go.dedis.ch/dela/core/ordering/cosipbft/blockstore"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/shuffle/neff"
	"io/ioutil"
	"testing"
)

func TestInitAction_Execute(t *testing.T) {

	ctx := node.Context{
		Injector: node.NewInjector(),
		Flags:    make(node.FlagSet),
		Out:      ioutil.Discard,
	}

	action := initAction{}

	err := action.Execute(ctx)
	require.EqualError(t, err, "failed to resolve shuffle: couldn't find dependency for 'shuffle.SHUFFLE'")

	ctx.Injector.Inject(neff.NewNeffShuffle(fake.Mino{}, fakeService{}, fakePool{}, &blockstore.InDisk{}, fake.NewSigner()))
	err = action.Execute(ctx)
	require.NoError(t, err)

}
