package binprefix

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/store"
	"go.dedis.ch/dela/core/store/hashtree"
	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestMerkleTree_IntegrationTest(t *testing.T) {
	db, clean := makeDB(t)
	defer clean()

	tree := NewMerkleTree(db, Nonce{})
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

	// Test read and write in a transaction.
	err = db.Update(func(txn kv.WritableTx) error {
		txtree := tree.WithTx(txn)

		err := txtree.Commit()
		require.NoError(t, err)

		for key := range values {
			path, err := txtree.GetPath(key[:])
			require.NoError(t, err)

			root, err := path.(Path).computeRoot(fac)
			require.NoError(t, err)
			require.Equal(t, root, tree.GetRoot())
			require.Equal(t, values[key], path.GetValue())
		}

		return nil
	})
	require.NoError(t, err)

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

	err = tree.Commit()
	require.NoError(t, err)

	db.View(func(txn kv.ReadableTx) error {
		b := txn.GetBucket(tree.bucket)
		require.NotNil(t, b)

		return b.ForEach(func(k, v []byte) error {
			t.Fatal("database should be empty")
			return nil
		})
	})
}

func TestMerkleTree_Random_IntegrationTest(t *testing.T) {
	f := func(nonce Nonce, n uint8, mem uint8) bool {
		t.Logf("Step nonce:%x n:%d mem:%d", nonce, n, mem%32)

		db, clean := makeDB(t)
		defer clean()

		tree := NewMerkleTree(db, nonce)
		tree.tree.memDepth = int(mem) % 32

		values := map[[4]byte][]byte{}

		var err error
		var stage hashtree.StagingTree = tree
		for k := 0; k < 2; k++ {
			stage, err = stage.Stage(func(snap store.Snapshot) error {
				for i := 0; i < int(n); i++ {
					key := [4]byte{}
					rand.Read(key[:])

					value := make([]byte, 4)
					rand.Read(value)

					values[key] = value

					snap.Set(key[:], value)
				}
				return nil
			})
			require.NoError(t, err)
		}

		// Test that all keys are present in-memory.
		for key, value := range values {
			v, err := stage.Get(key[:])
			require.NoError(t, err)
			require.Equal(t, value, v)
		}

		err = stage.Commit()
		require.NoError(t, err)

		tree = stage.(*MerkleTree)

		// Test that the keys are still present after persiting to the disk,
		// alongside with the computed hash for the nodes.
		for key, value := range values {
			path, err := tree.GetPath(key[:])
			require.NoError(t, err)
			require.Equal(t, value, path.GetValue())

			root, err := path.(Path).computeRoot(crypto.NewSha256Factory())
			require.NoError(t, err)
			require.Equal(t, tree.GetRoot(), root)
		}

		newTree := NewMerkleTree(db, nonce)
		err = newTree.Load()
		require.NoError(t, err)

		// Test that the keys are still present after loading the tree from the
		// disk.
		for key, value := range values {
			path, err := newTree.GetPath(key[:])
			require.NoError(t, err)
			require.Equal(t, value, path.GetValue())

			root, err := path.(Path).computeRoot(crypto.NewSha256Factory())
			require.NoError(t, err)
			require.Equal(t, newTree.GetRoot(), root)
		}

		return true
	}

	err := quick.Check(f, &quick.Config{MaxCount: 20})
	require.NoError(t, err)
}

func TestMerkleTree_Load(t *testing.T) {
	tree := NewMerkleTree(fakeDB{}, Nonce{})

	err := tree.Load()
	require.NoError(t, err)

	tree.tx = fakeTx{bucket: &fakeBucket{errScan: fake.GetError()}}
	err = tree.Load()
	require.EqualError(t, err, fake.Err("failed to load: while scanning"))

	tree.tx = nil
	tree.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	err = tree.Load()
	require.EqualError(t, err, fake.Err("while updating: failed to prepare: empty node failed"))
}

func TestMerkleTree_Get(t *testing.T) {
	tree := NewMerkleTree(fakeDB{}, Nonce{})

	err := tree.tree.Insert([]byte("ping"), []byte("pong"), &fakeBucket{})
	require.NoError(t, err)

	value, err := tree.Get([]byte("ping"))
	require.NoError(t, err)
	require.Equal(t, []byte("pong"), value)

	value, err = tree.Get([]byte("pong"))
	require.NoError(t, err)
	require.Nil(t, value)

	_, err = tree.Get(make([]byte, MaxDepth+1))
	require.EqualError(t, err, "couldn't search key: mismatch key length 33 > 32")

	tree.tx = wrongTx{}
	_, err = tree.Get([]byte{})
	require.EqualError(t, err,
		"couldn't search key: transaction 'binprefix.wrongTx' is not readable")
}

func TestMerkleTree_GetRoot(t *testing.T) {
	tree := NewMerkleTree(fakeDB{}, Nonce{})
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
	tree := NewMerkleTree(fakeDB{}, Nonce{})

	f := func(key [8]byte, value []byte) bool {
		err := tree.tree.Insert(key[:], value, &fakeBucket{})
		require.NoError(t, err)

		err = tree.tree.CalculateRoot(tree.hashFactory, &fakeBucket{})
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
	tree := NewMerkleTree(fakeDB{}, Nonce{})

	next, err := tree.Stage(func(snap store.Snapshot) error { return nil })
	require.NoError(t, err)
	require.NotEmpty(t, next.GetRoot())

	_, err = tree.Stage(func(store.Snapshot) error { return fake.GetError() })
	require.EqualError(t, err, fake.Err("callback failed"))

	tree.hashFactory = fake.NewHashFactory(fake.NewBadHash())
	_, err = tree.Stage(func(store.Snapshot) error { return nil })
	require.EqualError(t, err,
		fake.Err("couldn't update tree: failed to prepare: empty node failed"))

	tree.tx = badTx{}
	_, err = tree.Stage(func(store.Snapshot) error { return nil })
	require.EqualError(t, err, fake.Err("read bucket failed"))

	tree.tx = wrongTx{}
	_, err = tree.Stage(func(store.Snapshot) error { return nil })
	require.EqualError(t, err, "transaction 'binprefix.wrongTx' is not writable")
}

func TestMerkleTree_Commit(t *testing.T) {
	tree := NewMerkleTree(fakeDB{err: fake.GetError()}, Nonce{})

	err := tree.Commit()
	require.EqualError(t, err, fake.Err("failed to persist tree"))

	tree.tx = badTx{}
	err = tree.Commit()
	require.EqualError(t, err, fake.Err("failed to persist tree: read bucket failed"))
}

func TestWritableMerkleTree_Set(t *testing.T) {
	tree := writableMerkleTree{MerkleTree: NewMerkleTree(fakeDB{}, Nonce{})}

	err := tree.Set([]byte("ping"), []byte("pong"))
	require.NoError(t, err)
	require.Equal(t, 1, tree.tree.Len())

	err = tree.Set(make([]byte, MaxDepth+1), nil)
	require.EqualError(t, err, "couldn't insert pair: mismatch key length 33 > 32")
}

func TestWritableMerkleTree_Delete(t *testing.T) {
	tree := writableMerkleTree{MerkleTree: NewMerkleTree(fakeDB{}, Nonce{})}

	err := tree.Delete([]byte("ping"))
	require.NoError(t, err)

	err = tree.Delete(make([]byte, MaxDepth+1))
	require.EqualError(t, err, "couldn't delete key: mismatch key length 33 > 32")
}

// -----------------------------------------------------------------------------
// Utility functions

func makeDB(t *testing.T) (kv.DB, func()) {
	dir, err := os.MkdirTemp(os.TempDir(), "dela-pow")
	require.NoError(t, err)

	db, err := kv.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)

	return db, func() { os.RemoveAll(dir) }
}

type badTx struct {
	kv.WritableTx
}

func (tx badTx) GetBucketOrCreate([]byte) (kv.Bucket, error) {
	return nil, fake.GetError()
}

type wrongTx struct {
	store.Transaction
}
