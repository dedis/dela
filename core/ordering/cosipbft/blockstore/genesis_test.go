package blockstore

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/ordering/cosipbft/roster"
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestCachedGenesis_Get(t *testing.T) {
	store := NewGenesisStore().(*cachedGenesis)

	ro := roster.FromAuthority(fake.NewAuthority(3, fake.NewSigner))

	_, err := store.Get()
	require.EqualError(t, err, "missing genesis block")

	block, err := types.NewGenesis(ro)
	require.NoError(t, err)

	store.set = true
	store.genesis = block

	genesis, err := store.Get()
	require.NoError(t, err)
	require.Equal(t, block, genesis)
}

func TestCachedGenesis_Set(t *testing.T) {
	ro := roster.FromAuthority(fake.NewAuthority(3, fake.NewSigner))

	block, err := types.NewGenesis(ro)
	require.NoError(t, err)

	store := NewGenesisStore().(*cachedGenesis)

	err = store.Set(block)
	require.NoError(t, err)
	require.True(t, store.set)

	err = store.Set(block)
	require.EqualError(t, err, "genesis block is already set")
}

func TestCachedGenesis_Exists(t *testing.T) {
	store := NewGenesisStore()

	require.False(t, store.Exists())

	store.Set(types.Genesis{})
	require.True(t, store.Exists())
}
