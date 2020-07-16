package mem

import (
	"bytes"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/hashtree"
	"go.dedis.ch/dela/internal/testing/fake"
	"golang.org/x/xerrors"
)

func TestMerkleTree_Random(t *testing.T) {
	var tree hashtree.Tree = NewMerkleTree()
	keys := make([][MaxDepth]byte, 0)

	f := func(key [MaxDepth]byte, value []byte) bool {
		var err error
		tree, err = tree.Stage(func(snap store.Snapshot) error {
			return snap.Set(key[:], value)
		})

		require.NoError(t, err)

		keys = append(keys, key)

		path, err := tree.GetPath(key[:])
		require.NoError(t, err)

		return bytes.Equal(value, path.GetValue())
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)

	tree, err = tree.Stage(func(snap store.Snapshot) error {
		for _, key := range keys {
			err = snap.Delete(key[:])
			require.NoError(t, err)
		}

		return nil
	})
	require.NoError(t, err)
	require.Len(t, tree.GetRoot(), 32)
}

func TestMerkleTree_Get(t *testing.T) {
	tree := NewMerkleTree()

	err := tree.tree.Insert([]byte("ping"), []byte("pong"))
	require.NoError(t, err)

	value, err := tree.Get([]byte("ping"))
	require.NoError(t, err)
	require.Equal(t, []byte("pong"), value)

	value, err = tree.Get([]byte("pong"))
	require.NoError(t, err)
	require.Nil(t, value)

	_, err = tree.Get(make([]byte, MaxDepth+1))
	require.EqualError(t, err, "couldn't search key: mismatch key length 33 > 32")
}

func TestMerkleTree_GetRoot(t *testing.T) {
	tree := NewMerkleTree()
	// Tree is not yet updated.
	require.Empty(t, tree.GetRoot())

	empty, err := tree.Stage(func(store.Snapshot) error { return nil })
	require.NoError(t, err)
	// Tree is updated even without any leafs.
	require.NotEmpty(t, empty.GetRoot())

	filled, err := tree.Stage(func(snap store.Snapshot) error {
		return snap.Set([]byte("ping"), []byte("pong"))
	})
	require.NoError(t, err)
	require.NotEmpty(t, filled.GetRoot())
	require.NotEqual(t, empty.GetRoot(), filled.GetRoot())
}

func TestMerkleTree_GetPath(t *testing.T) {
	tree := NewMerkleTree()

	f := func(key [8]byte, value []byte) bool {
		err := tree.tree.Insert(key[:], value)
		require.NoError(t, err)

		err = tree.tree.Update(tree.hashFactory)
		require.NoError(t, err)

		path, err := tree.GetPath(key[:])
		require.NoError(t, err)

		root, err := computeRoot(path.(Path).leaf.GetHash(), key[:], path.(Path).interiors, tree.hashFactory)
		require.NoError(t, err)
		require.Equal(t, root, path.GetRoot())

		return bytes.Equal(value, path.GetValue())
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)

	_, err = tree.GetPath(make([]byte, MaxDepth+1))
	require.EqualError(t, err, "couldn't search key: mismatch key length 33 > 32")
}

func TestMerkleTree_Stage(t *testing.T) {
	tree := NewMerkleTree()

	next, err := tree.Stage(func(snap store.Snapshot) error { return nil })
	require.NoError(t, err)
	require.NotEmpty(t, next.GetRoot())

	_, err = tree.Stage(func(store.Snapshot) error { return xerrors.New("oops") })
	require.EqualError(t, err, "callback failed: oops")

	tree.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	_, err = tree.Stage(func(store.Snapshot) error { return nil })
	require.EqualError(t, err,
		"couldn't update tree: failed to prepare: failed to write: fake error")
}

func TestWritableMerkleTree_Set(t *testing.T) {
	tree := writableMerkleTree{MerkleTree: NewMerkleTree()}

	err := tree.Set([]byte("ping"), []byte("pong"))
	require.NoError(t, err)
	require.Equal(t, 1, tree.tree.Len())

	err = tree.Set(make([]byte, MaxDepth+1), nil)
	require.EqualError(t, err, "couldn't insert pair: mismatch key length 33 > 32")
}

func TestWritableMerkleTree_Delete(t *testing.T) {
	tree := writableMerkleTree{MerkleTree: NewMerkleTree()}

	err := tree.Delete([]byte("ping"))
	require.NoError(t, err)

	err = tree.Delete(make([]byte, MaxDepth+1))
	require.EqualError(t, err, "couldn't delete key: mismatch key length 33 > 32")
}
