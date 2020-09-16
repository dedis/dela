package tree

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestAddrSet_Search(t *testing.T) {
	set := AddrSet{
		fake.NewAddress(0): {},
		fake.NewAddress(1): {},
		fake.NewAddress(2): {},
	}

	require.True(t, set.Search(fake.NewAddress(0)))
	require.True(t, set.Search(fake.NewAddress(2)))
	require.False(t, set.Search(fake.NewAddress(5)))
}

func TestBranches_Search(t *testing.T) {
	branches := Branches{
		fake.NewAddress(0): AddrSet{fake.NewAddress(1): {}},
		fake.NewAddress(3): AddrSet{fake.NewAddress(2): {}},
	}

	require.Equal(t, fake.NewAddress(0), branches.Search(fake.NewAddress(0)))
	require.Equal(t, fake.NewAddress(0), branches.Search(fake.NewAddress(1)))
	require.Equal(t, fake.NewAddress(3), branches.Search(fake.NewAddress(3)))
	require.Equal(t, fake.NewAddress(3), branches.Search(fake.NewAddress(2)))
	require.Nil(t, branches.Search(fake.NewAddress(5)))
}

func TestDynTree_GetMaxHeight(t *testing.T) {
	tree := NewTree(3, makeAddrs(4))

	require.Equal(t, 3, tree.GetMaxHeight())
}

func TestDynTree_GetRoute(t *testing.T) {
	addrs := makeAddrs(200)
	tree := NewTree(5, addrs)

	for _, addr := range addrs {
		gateway, err := tree.GetRoute(fake.NewAddress(500))
		require.NoError(t, err)
		require.Nil(t, gateway)

		gateway, err = tree.GetRoute(addr)
		require.NoError(t, err)
		require.NotNil(t, gateway)
	}
}

func TestDynTree_GetChildren(t *testing.T) {
	tree := NewTree(3, makeAddrs(20))
	require.Empty(t, tree.GetChildren(fake.NewAddress(0)))

	tree.GetRoute(fake.NewAddress(0))
	require.Len(t, tree.GetChildren(fake.NewAddress(0)), 3)
}

func TestDynTree_Remove(t *testing.T) {
	tree := NewTree(3, makeAddrs(20)).(*dynTree)
	tree.GetRoute(fake.NewAddress(1))

	tree.Remove(fake.NewAddress(1))
	require.Len(t, tree.offline, 1)
	require.Len(t, tree.branches, 1)

	tree = NewTree(3, makeAddrs(1)).(*dynTree)
	tree.GetRoute(fake.NewAddress(0))

	tree.Remove(fake.NewAddress(0))
	require.Len(t, tree.offline, 1)
	require.Len(t, tree.branches, 0)
}
