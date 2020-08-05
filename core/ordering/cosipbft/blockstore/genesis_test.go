package blockstore

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestCachedGenesis_Get(t *testing.T) {
	store := NewGenesisStore().(*cachedGenesis)

	_, err := store.Get()
	require.EqualError(t, err, "missing genesis block")

	block, err := types.NewGenesis(fake.NewAuthority(3, fake.NewSigner))
	require.NoError(t, err)

	store.set = true
	store.genesis = block

	genesis, err := store.Get()
	require.NoError(t, err)
	require.Equal(t, block, genesis)
}

func TestCachedGenesis_Set(t *testing.T) {
	block, err := types.NewGenesis(fake.NewAuthority(3, fake.NewSigner))
	require.NoError(t, err)

	store := NewGenesisStore().(*cachedGenesis)

	err = store.Set(block)
	require.NoError(t, err)
	require.True(t, store.set)

	err = store.Set(block)
	require.EqualError(t, err, "genesis block is already set")
}
