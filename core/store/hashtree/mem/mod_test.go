package mem

import (
	"bytes"
	"crypto/rand"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/internal/testing/fake"
	"golang.org/x/xerrors"
)

func TestMerkleTree_IntegrationTests(t *testing.T) {
	db, clean := makeDB(t)
	defer clean()

	tree := NewMerkleTree(db)
	tree.tree.memDepth = 5
	fac := tree.hashFactory
	values := map[[MaxDepth]byte][]byte{}

	next, err := tree.Stage(func(snap store.Snapshot) error {
		for i := 0; i < 1000; i++ {
			key := [MaxDepth]byte{}
			rand.Read(key[:])

			value := make([]byte, 8)
			rand.Read(value)

			err := snap.Set(key[:], value)
			require.NoError(t, err)

			values[key] = value
		}
		return nil
	})

	require.NoError(t, err)
	require.NotEmpty(t, values)
	tree = next.(*MerkleTree)

	_, err = tree.Commit()
	require.NoError(t, err)

	for key := range values {
		path, err := tree.GetPath(key[:])
		require.NoError(t, err)

		root, err := path.(Path).computeRoot(fac)
		require.NoError(t, err)
		require.Equal(t, root, tree.GetRoot())
		require.Equal(t, values[key], path.GetValue())
	}

	next, err = tree.Stage(func(snap store.Snapshot) error {
		for key := range values {
			err = snap.Delete(key[:])
			require.NoError(t, err)
		}

		return nil
	})
	require.NoError(t, err)
	tree = next.(*MerkleTree)

	require.Len(t, tree.GetRoot(), 32)
	require.IsType(t, (*EmptyNode)(nil), tree.tree.root)

	_, err = tree.Commit()
	require.NoError(t, err)

	// TODO: check db to be empty
}

func TestMerkleTree_Get(t *testing.T) {
	tree := NewMerkleTree(fakeDB{})

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
	tree := NewMerkleTree(fakeDB{})
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
	tree := NewMerkleTree(fakeDB{})

	f := func(key [8]byte, value []byte) bool {
		err := tree.tree.Insert(key[:], value)
		require.NoError(t, err)

		err = tree.tree.Update(tree.hashFactory)
		require.NoError(t, err)

		path, err := tree.GetPath(key[:])
		require.NoError(t, err)

		root, err := path.(Path).computeRoot(tree.hashFactory)
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
	tree := NewMerkleTree(fakeDB{})

	next, err := tree.Stage(func(snap store.Snapshot) error { return nil })
	require.NoError(t, err)
	require.NotEmpty(t, next.GetRoot())

	_, err = tree.Stage(func(store.Snapshot) error { return xerrors.New("oops") })
	require.EqualError(t, err, "callback failed: oops")

	tree.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	_, err = tree.Stage(func(store.Snapshot) error { return nil })
	require.EqualError(t, err,
		"couldn't update tree: failed to prepare: empty node failed: fake error")

	tree.tree.db = fakeDB{err: xerrors.New("oops")}
	_, err = tree.Stage(func(store.Snapshot) error { return nil })
	require.EqualError(t, err, "failed to create bucket: oops")
}

func TestMerkleTree_Commit(t *testing.T) {
	tree := NewMerkleTree(fakeDB{err: xerrors.New("oops")})

	_, err := tree.Commit()
	require.EqualError(t, err, "failed to persist tree: oops")
}

func TestWritableMerkleTree_Set(t *testing.T) {
	tree := writableMerkleTree{MerkleTree: NewMerkleTree(fakeDB{})}

	err := tree.Set([]byte("ping"), []byte("pong"))
	require.NoError(t, err)
	require.Equal(t, 1, tree.tree.Len())

	err = tree.Set(make([]byte, MaxDepth+1), nil)
	require.EqualError(t, err, "couldn't insert pair: mismatch key length 33 > 32")
}

func TestWritableMerkleTree_Delete(t *testing.T) {
	tree := writableMerkleTree{MerkleTree: NewMerkleTree(fakeDB{})}

	err := tree.Delete([]byte("ping"))
	require.NoError(t, err)

	err = tree.Delete(make([]byte, MaxDepth+1))
	require.EqualError(t, err, "couldn't delete key: mismatch key length 33 > 32")
}

// Utility functions -----------------------------------------------------------

func makeDB(t *testing.T) (kv.DB, func()) {
	dir, err := ioutil.TempDir(os.TempDir(), "dela-pow")
	require.NoError(t, err)

	db, err := kv.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)

	return db, func() { os.RemoveAll(dir) }
}
