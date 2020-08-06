package blockstore

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/validation/simple"
)

func TestInMemory_Len(t *testing.T) {
	store := NewInMemory()
	require.Equal(t, uint64(0), store.Len())

	store.blocks = []types.BlockLink{{}, {}}
	require.Equal(t, uint64(2), store.Len())
}

func TestInMemory_Store(t *testing.T) {
	store := NewInMemory()

	err := store.Store(makeLink(t, types.Digest{}))
	require.NoError(t, err)

	err = store.Store(makeLink(t, store.blocks[0].GetTo().GetHash()))
	require.NoError(t, err)

	err = store.Store(makeLink(t, types.Digest{}))
	require.EqualError(t, err, "mismatch link '00000000' != '2c34ce1d'")
}

func TestInMemory_Get(t *testing.T) {
	store := NewInMemory()

	store.blocks = []types.BlockLink{makeLink(t, types.Digest{})}

	block, err := store.Get(store.blocks[0].GetTo().GetHash())
	require.NoError(t, err)
	require.Equal(t, store.blocks[0], block)

	_, err = store.Get(types.Digest{})
	require.EqualError(t, err, "block not found: no block")
}

func TestInMemory_Last(t *testing.T) {
	store := NewInMemory()

	_, err := store.Last()
	require.EqualError(t, err, "store empty: no block")

	store.blocks = []types.BlockLink{makeLink(t, types.Digest{})}
	block, err := store.Last()
	require.NoError(t, err)
	require.Equal(t, store.blocks[0], block)
}

// Utility functions -----------------------------------------------------------

func makeLink(t *testing.T, from types.Digest) types.BlockLink {
	to, err := types.NewBlock(simple.NewData(nil))
	require.NoError(t, err)

	link := types.NewBlockLink(from, to, nil, nil)

	return link
}
