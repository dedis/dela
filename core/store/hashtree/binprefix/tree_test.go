package binprefix

import (
	"math"
	"math/big"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/internal/testing/fake"
	"golang.org/x/xerrors"
)

func init() {
	nodeFormats.Register(fake.BadFormat, fake.NewBadFormat())
}

func TestTree_Len(t *testing.T) {
	tree := NewTree(Nonce{})
	require.Equal(t, 0, tree.Len())

	tree.root = NewInteriorNode(0, big.NewInt(0))
	require.Equal(t, 0, tree.Len())

	tree.root.(*InteriorNode).left = NewLeafNode(1, nil, nil)
	require.Equal(t, 1, tree.Len())

	tree.root.(*InteriorNode).right = NewLeafNode(1, nil, nil)
	require.Equal(t, 2, tree.Len())
}

func TestTree_Load(t *testing.T) {
	tree := NewTree(Nonce{})

	bucket := makeBucket(t)

	err := tree.FillFromBucket(bucket)
	require.NoError(t, err)

	tree.factory = fake.NewBadMessageFactory()
	err = tree.FillFromBucket(bucket)
	require.EqualError(t, err, fake.Err("while scanning: tree node malformed"))
}

func TestTree_Search(t *testing.T) {
	tree := NewTree(Nonce{})

	value, err := tree.Search(nil, nil, &fakeBucket{})
	require.NoError(t, err)
	require.Nil(t, value)

	tree.root = NewLeafNode(0, makeKey([]byte("A")), []byte("B"))
	value, err = tree.Search([]byte("A"), nil, &fakeBucket{})
	require.NoError(t, err)
	require.Equal(t, []byte("B"), value)

	_, err = tree.Search(make([]byte, MaxDepth+1), nil, &fakeBucket{})
	require.EqualError(t, err, "mismatch key length 33 > 32")

	tree.root = fakeNode{err: fake.GetError()}
	_, err = tree.Search([]byte("A"), nil, nil)
	require.EqualError(t, err, fake.Err("failed to search"))
}

func TestTree_Insert(t *testing.T) {
	tree := NewTree(Nonce{})

	err := tree.Insert([]byte("ping"), []byte("pong"), &fakeBucket{})
	require.NoError(t, err)
	require.Equal(t, 1, tree.Len())

	err = tree.Insert(make([]byte, MaxDepth+1), nil, &fakeBucket{})
	require.EqualError(t, err, "mismatch key length 33 > 32")

	tree.root = fakeNode{err: fake.GetError()}
	err = tree.Insert([]byte("A"), []byte("B"), nil)
	require.EqualError(t, err, fake.Err("failed to insert"))
}

func TestTree_Insert_UpdateLeaf(t *testing.T) {
	bucket := &fakeBucket{}
	hashFactory := crypto.NewSha256Factory()
	tree := NewTree(Nonce{})

	for i := 0; i <= math.MaxUint8; i++ {
		key := []byte{byte(i)}
		err := tree.Insert(key[:], key[:], bucket)
		require.NoError(t, err)
	}

	err := tree.CalculateRoot(hashFactory, bucket)
	require.NoError(t, err)

	updateKey := makeKey([]byte{byte(math.MaxUint8 - 1)})

	var existingHash []byte
	tree.root.Visit(func(node TreeNode) error {
		lnode, ok := node.(*LeafNode)
		if ok && lnode.key.Cmp(updateKey) == 0 {
			existingHash = lnode.GetHash()
		}

		return nil
	})

	require.NotEmpty(t, existingHash)

	err = tree.Persist(bucket)
	require.NoError(t, err)

	clone := tree.Clone()

	err = clone.Insert([]byte{byte(math.MaxUint8 - 1)}, []byte{0}, bucket)
	require.NoError(t, err)

	err = clone.CalculateRoot(hashFactory, bucket)
	require.NoError(t, err)

	var updatedHash []byte
	clone.root.Visit(func(node TreeNode) error {
		lnode, ok := node.(*LeafNode)
		if ok && lnode.key.Cmp(updateKey) == 0 {
			updatedHash = lnode.GetHash()
		}
		return nil
	})

	require.NotEmpty(t, updatedHash)
	require.NotEqual(t, existingHash, updatedHash)
}

func TestTree_Delete(t *testing.T) {
	tree := NewTree(Nonce{})

	err := tree.Delete(nil, &fakeBucket{})
	require.NoError(t, err)

	err = tree.Delete([]byte("ping"), &fakeBucket{})
	require.NoError(t, err)

	tree.root = NewLeafNode(0, makeKey([]byte("ping")), []byte("pong"))
	err = tree.Delete([]byte("ping"), &fakeBucket{})
	require.NoError(t, err)
	require.IsType(t, (*EmptyNode)(nil), tree.root)

	err = tree.Delete(make([]byte, MaxDepth+1), &fakeBucket{})
	require.EqualError(t, err, "mismatch key length 33 > 32")

	tree.root = fakeNode{err: fake.GetError()}
	err = tree.Delete([]byte("A"), nil)
	require.EqualError(t, err, fake.Err("failed to delete"))
}

func TestTree_Persist(t *testing.T) {
	bucket := &fakeBucket{}

	tree := NewTree(Nonce{})

	// 1. Test if the tree can be stored in disk with the default parameter to
	// only store leaf nodes.
	for i := 0; i <= math.MaxUint8; i++ {
		key := []byte{byte(i)}

		err := tree.Insert(key[:], key[:], bucket)
		require.NoError(t, err)
	}

	err := tree.Persist(bucket)
	require.NoError(t, err)

	count := 0
	for _, value := range bucket.values {
		require.Regexp(t, `{"Leaf":{[^}]+}}`, string(value))
		count++
	}
	require.Equal(t, 256, count)

	// 2. Test with a memory depth of 6 which means that only disk nodes should
	// exist after this level.
	tree.memDepth = 6
	err = tree.Persist(bucket)
	require.NoError(t, err)

	tree.root.Visit(func(n TreeNode) error {
		switch node := n.(type) {
		case *InteriorNode:
			require.LessOrEqual(t, int(node.depth), 6)
		case *EmptyNode:
			require.LessOrEqual(t, int(node.depth), 6)
		case *LeafNode:
			t.Fatal("no leaf node expected in-memory")
		}
		return nil
	})

	count = 0
	for _, value := range bucket.values {
		require.Regexp(t, `{"(Leaf|Interior)":{[^}]+}}`, string(value))
		count++
	}
	// 2^9-1 - (2^7-1) = 384
	require.Equal(t, 384, count)

	// 3. Test with only the root in-memory.
	tree.memDepth = 0
	err = tree.Persist(bucket)
	require.NoError(t, err)
	// 2^9-1 = 511 (root is not on disk)
	require.Equal(t, 510, len(bucket.values))

	tree.root.Visit(func(n TreeNode) error {
		switch node := n.(type) {
		case *InteriorNode:
			// Only the root is allow as an interior node.
			require.Equal(t, uint16(0), node.depth)
		case *EmptyNode:
			t.Fatal("expect only disk nodes")
		case *LeafNode:
			t.Fatal("expect only disk nodes")
		}
		return nil
	})

	tree.root = NewInteriorNode(0, big.NewInt(0))

	err = tree.Persist(&fakeBucket{errSet: fake.GetError()})
	require.EqualError(t, err,
		fake.Err("visiting empty: failed to store node: failed to set key"))

	err = tree.Persist(&fakeBucket{errScan: fake.GetError()})
	require.EqualError(t, err,
		fake.Err("visiting empty: failed to clean subtree"))
}

func TestTree_Clone(t *testing.T) {
	tree := NewTree(Nonce{})

	f := func(key [8]byte, value []byte) bool {
		return tree.Insert(key[:], value, &fakeBucket{}) == nil
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)

	clone := tree.Clone()
	require.Equal(t, tree.Len(), clone.Len())

	require.NoError(t, tree.CalculateRoot(crypto.NewSha256Factory(), &fakeBucket{}))
	require.NoError(t, clone.CalculateRoot(crypto.NewSha256Factory(), &fakeBucket{}))
	require.Equal(t, tree.root.GetHash(), clone.root.GetHash())
}

func TestEmptyNode_GetHash(t *testing.T) {
	node := NewEmptyNode(0, big.NewInt(0))
	require.Empty(t, node.GetHash())

	node.hash = []byte("ping")
	require.Equal(t, []byte("ping"), node.GetHash())
}

func TestEmptyNode_GetType(t *testing.T) {
	node := NewEmptyNode(0, big.NewInt(0))
	require.Equal(t, emptyNodeType, node.GetType())
}

func TestEmptyNode_Search(t *testing.T) {
	node := NewEmptyNode(0, big.NewInt(0))
	path := newPath([]byte{}, nil)

	value, err := node.Search(new(big.Int), &path, nil)
	require.NoError(t, err)
	require.Nil(t, value)
	require.Nil(t, path.value)
}

func TestEmptyNode_Insert(t *testing.T) {
	node := NewEmptyNode(0, big.NewInt(0))

	next, err := node.Insert(big.NewInt(0), []byte("pong"), nil)
	require.NoError(t, err)
	require.IsType(t, (*LeafNode)(nil), next)
}

func TestEmptyNode_Delete(t *testing.T) {
	node := NewEmptyNode(0, big.NewInt(0))

	next, err := node.Delete(nil, nil)
	require.NoError(t, err)
	require.Same(t, node, next)
}

func TestEmptyNode_Prepare(t *testing.T) {
	node := NewEmptyNode(3, big.NewInt(0))

	fac := crypto.NewSha256Factory()

	hash, err := node.Prepare([]byte("nonce"), new(big.Int), nil, fac)
	require.NoError(t, err)
	require.Equal(t, hash, node.hash)

	calls := &fake.Call{}
	node.hash = nil
	_, err = node.Prepare([]byte{1}, new(big.Int).SetBytes([]byte{2}), nil, fake.NewHashFactory(&fake.Hash{Call: calls}))
	require.NoError(t, err)
	require.Equal(t, 1, calls.Len())
	require.Equal(t, "\x00\x01\x02\x03\x00", string(calls.Get(0, 0).([]byte)))

	node.hash = nil
	_, err = node.Prepare([]byte{1}, new(big.Int), nil, fake.NewHashFactory(fake.NewBadHash()))
	require.EqualError(t, err, fake.Err("empty node failed"))
}

func TestEmptyNode_Visit(t *testing.T) {
	node := NewEmptyNode(3, big.NewInt(0))
	count := 0

	node.Visit(func(tn TreeNode) error {
		require.Same(t, node, tn)
		count++

		return nil
	})

	require.Equal(t, 1, count)
}

func TestEmptyNode_Clone(t *testing.T) {
	node := NewEmptyNode(3, big.NewInt(0))

	clone := node.Clone()
	require.Equal(t, node, clone)
}

func TestEmptyNode_Serialize(t *testing.T) {
	node := NewEmptyNode(3, big.NewInt(1))

	data, err := node.Serialize(testCtx)
	require.Equal(t, `{"Empty":{"Digest":"","Depth":3,"Prefix":"AQ=="}}`, string(data))
	require.NoError(t, err)

	_, err = node.Serialize(fake.NewBadContext())
	require.EqualError(t, err, fake.Err("failed to encode empty node"))
}

func TestInteriorNode_GetHash(t *testing.T) {
	node := NewInteriorNode(3, big.NewInt(0))

	require.Empty(t, node.GetHash())

	node.hash = []byte{1, 2, 3}
	require.Equal(t, []byte{1, 2, 3}, node.GetHash())
}

func TestInteriorNode_GetType(t *testing.T) {
	node := NewInteriorNode(3, big.NewInt(0))

	require.Equal(t, interiorNodeType, node.GetType())
}

func TestInteriorNode_Search(t *testing.T) {
	node := NewInteriorNode(0, big.NewInt(0))
	node.left = NewLeafNode(1, big.NewInt(0), []byte("ping"))
	node.right = NewLeafNode(1, big.NewInt(1), []byte("pong"))

	path := newPath([]byte{}, nil)
	value, err := node.Search(big.NewInt(0), &path, nil)
	require.NoError(t, err)
	require.Equal(t, "ping", string(value))
	require.Len(t, path.interiors, 1)

	path = newPath([]byte{}, nil)
	value, err = node.Search(big.NewInt(1), &path, nil)
	require.NoError(t, err)
	require.Equal(t, "pong", string(value))
	require.Len(t, path.interiors, 1)
}

func TestInteriorNode_Search_Error(t *testing.T) {
	node := NewInteriorNode(0, big.NewInt(0))
	node.left = NewDiskNode(0, nil, testCtx, NodeFactory{})

	_, err := node.Search(big.NewInt(0), nil, &fakeBucket{})
	require.EqualError(t, err, "failed to load node: prefix 0 (depth 0) not in database")

	node.left = nil
	node.right = NewDiskNode(0, nil, testCtx, NodeFactory{})

	_, err = node.Search(big.NewInt(1), nil, &fakeBucket{})
	require.EqualError(t, err, "failed to load node: prefix 1 (depth 0) not in database")
}

func TestInteriorNode_Insert(t *testing.T) {
	node := NewInteriorNode(0, big.NewInt(0))

	next, err := node.Insert(big.NewInt(0), []byte("ping"), nil)
	require.NoError(t, err)
	require.Same(t, node, next)

	next, err = node.Insert(big.NewInt(1), []byte("pong"), nil)
	require.NoError(t, err)
	require.Same(t, node, next)
}

func TestInteriorNode_Delete(t *testing.T) {
	node := NewInteriorNode(0, big.NewInt(0))
	node.left = NewLeafNode(1, big.NewInt(0), []byte("ping"))
	node.right = NewLeafNode(1, big.NewInt(1), []byte("pong"))

	next, err := node.Delete(big.NewInt(0), nil)
	require.NoError(t, err)
	require.Same(t, node, next)

	next, err = node.Delete(big.NewInt(1), nil)
	require.NoError(t, err)
	require.IsType(t, (*EmptyNode)(nil), next)

	node.right = NewDiskNode(1, nil, testCtx, NodeFactory{})
	_, err = node.Delete(big.NewInt(0), &fakeBucket{})
	require.EqualError(t, err, "failed to load node: prefix 1 (depth 1) not in database")

	node.right = NewEmptyNode(1, big.NewInt(2))
	node.left = NewDiskNode(1, nil, testCtx, NodeFactory{})
	_, err = node.Delete(big.NewInt(1), &fakeBucket{})
	require.EqualError(t, err, "failed to load node: prefix 0 (depth 1) not in database")
}

func TestInteriorNode_Prepare(t *testing.T) {
	node := NewInteriorNode(1, big.NewInt(0))
	node.left = fakeNode{data: []byte{0xaa}}
	node.right = fakeNode{data: []byte{0xbb}}
	calls := &fake.Call{}

	_, err := node.Prepare([]byte{1}, big.NewInt(1), nil, fake.NewHashFactory(&fake.Hash{Call: calls}))
	require.NoError(t, err)
	require.Equal(t, 1, calls.Len())
	require.Equal(t, "\xaa\xbb", string(calls.Get(0, 0).([]byte)))

	node.hash = nil
	node.left = fakeNode{err: xerrors.New("bad node error")}
	_, err = node.Prepare([]byte{1}, big.NewInt(2), nil, crypto.NewSha256Factory())
	require.EqualError(t, err, "bad node error")

	node.left = fakeNode{}
	node.right = fakeNode{err: xerrors.New("bad node error")}
	_, err = node.Prepare([]byte{1}, big.NewInt(2), nil, crypto.NewSha256Factory())
	require.EqualError(t, err, "bad node error")

	node.right = fakeNode{}
	_, err = node.Prepare([]byte{1}, big.NewInt(2), nil, fake.NewHashFactory(fake.NewBadHash()))
	require.EqualError(t, err, fake.Err("interior node failed"))
}

func TestInteriorNode_Visit(t *testing.T) {
	node := NewInteriorNode(0, big.NewInt(0))

	counter := 0
	err := node.Visit(func(n TreeNode) error {
		if counter == 2 {
			require.IsType(t, node, n)
		} else {
			require.IsType(t, (*EmptyNode)(nil), n)
		}
		counter++

		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 3, counter)

	node.left = fakeNode{}
	node.right = fakeNode{}
	err = node.Visit(func(TreeNode) error { return fake.GetError() })
	require.EqualError(t, err, fake.Err("visiting interior"))

	node.right = NewEmptyNode(1, big.NewInt(2))
	err = node.Visit(func(TreeNode) error { return fake.GetError() })
	require.EqualError(t, err, fake.Err("visiting empty"))
}

func TestInteriorNode_Clone(t *testing.T) {
	node := NewInteriorNode(0, big.NewInt(0))

	clone := node.Clone()
	require.Equal(t, node, clone)
}

func TestInteriorNode_Serialize(t *testing.T) {
	node := NewInteriorNode(2, big.NewInt(3))

	data, err := node.Serialize(testCtx)
	require.NoError(t, err)
	require.Equal(t, `{"Interior":{"Digest":"","Depth":2,"Prefix":"Aw=="}}`, string(data))

	_, err = node.Serialize(fake.NewBadContext())
	require.EqualError(t, err, fake.Err("failed to encode interior node"))
}

func TestLeafNode_GetHash(t *testing.T) {
	node := NewLeafNode(0, makeKey([]byte("ping")), []byte("pong"))

	require.Empty(t, node.GetHash())

	_, err := node.Prepare([]byte{1}, big.NewInt(2), nil, crypto.NewSha256Factory())
	require.NoError(t, err)
	require.Len(t, node.GetHash(), 32)
}

func TestLeafNode_GetType(t *testing.T) {
	node := NewLeafNode(0, makeKey([]byte("ping")), []byte("pong"))

	require.Equal(t, leafNodeType, node.GetType())
}

func TestLeafNode_Search(t *testing.T) {
	node := NewLeafNode(0, makeKey([]byte("ping")), []byte("pong"))
	path := newPath([]byte{}, []byte("ping"))

	value, err := node.Search(makeKey([]byte("ping")), &path, nil)
	require.NoError(t, err)
	require.Equal(t, []byte("pong"), value)
	require.Equal(t, []byte("pong"), path.value)

	value, err = node.Search(makeKey([]byte("pong")), nil, nil)
	require.NoError(t, err)
	require.Nil(t, value)
}

func TestLeafNode_Insert(t *testing.T) {
	node := NewLeafNode(0, makeKey([]byte("ping")), []byte("pong"))

	next, err := node.Insert(makeKey([]byte("ping")), []byte("abc"), nil)
	require.NoError(t, err)
	require.Same(t, node, next)
	require.Equal(t, []byte("abc"), next.(*LeafNode).value)

	node = NewLeafNode(0, makeKey([]byte{0}), []byte{0xaa})
	next, err = node.Insert(makeKey([]byte{1}), []byte{0xbb}, nil)
	require.NoError(t, err)
	require.IsType(t, (*InteriorNode)(nil), next)
	require.IsType(t, (*LeafNode)(nil), next.(*InteriorNode).left)
	require.IsType(t, (*LeafNode)(nil), next.(*InteriorNode).right)

	node = NewLeafNode(0, makeKey([]byte{1}), []byte{0xaa})
	next, err = node.Insert(makeKey([]byte{0}), []byte{0xbb}, nil)
	require.NoError(t, err)
	require.IsType(t, (*InteriorNode)(nil), next)
	require.IsType(t, (*LeafNode)(nil), next.(*InteriorNode).left)
	require.IsType(t, (*LeafNode)(nil), next.(*InteriorNode).right)
}

func TestLeafNode_Delete(t *testing.T) {
	node := NewLeafNode(0, makeKey([]byte("ping")), []byte("pong"))

	next, err := node.Delete(makeKey([]byte("pong")), nil)
	require.NoError(t, err)
	require.Same(t, node, next)

	next, err = node.Delete(makeKey([]byte("ping")), nil)
	require.NoError(t, err)
	require.IsType(t, (*EmptyNode)(nil), next)
}

func TestLeafNode_Prepare(t *testing.T) {
	node := NewLeafNode(3, makeKey([]byte("ping")), []byte("pong"))
	calls := &fake.Call{}

	_, err := node.Prepare([]byte{1}, big.NewInt(2), nil, fake.NewHashFactory(&fake.Hash{Call: calls}))
	require.NoError(t, err)
	require.Equal(t, 1, calls.Len())
	require.Equal(t, "\x02\x01\x03\x00\x02pingpong", string(calls.Get(0, 0).([]byte)))

	node.hash = nil
	_, err = node.Prepare(nil, big.NewInt(0), nil, fake.NewHashFactory(fake.NewBadHash()))
	require.EqualError(t, err, fake.Err("leaf node failed"))
}

func TestLeafNode_Visit(t *testing.T) {
	node := NewLeafNode(3, makeKey([]byte("ping")), []byte("pong"))

	counter := 0
	err := node.Visit(func(n TreeNode) error {
		counter++
		require.IsType(t, node, n)

		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, counter)

	err = node.Visit(func(n TreeNode) error { return fake.GetError() })
	require.EqualError(t, err, fake.Err("visiting leaf"))
}

func TestLeafNode_Clone(t *testing.T) {
	node := NewLeafNode(3, makeKey([]byte("ping")), []byte("pong"))

	clone := node.Clone()
	require.Equal(t, node, clone)
}

func TestLeafNode_Serialize(t *testing.T) {
	node := NewLeafNode(2, big.NewInt(2), []byte{0xaa})

	data, err := node.Serialize(testCtx)
	require.NoError(t, err)
	require.Equal(t, `{"Leaf":{"Digest":"","Depth":2,"Prefix":"Ag==","Value":"qg=="}}`, string(data))

	_, err = node.Serialize(fake.NewBadContext())
	require.EqualError(t, err, fake.Err("failed to encode leaf node"))
}

func TestNodeFactory_Deserialize(t *testing.T) {
	fac := NodeFactory{}

	msg, err := fac.Deserialize(testCtx, []byte(`{"Empty":{}}`))
	require.NoError(t, err)
	require.IsType(t, &EmptyNode{}, msg)

	_, err = fac.Deserialize(testCtx, []byte(`{}`))
	require.EqualError(t, err, "format failed: message is empty")
}

// -----------------------------------------------------------------------------
// Utility functions

func makeBucket(t *testing.T) *fakeBucket {
	emptyNode, err := NewEmptyNode(1, big.NewInt(0)).Serialize(testCtx)
	require.NoError(t, err)

	leafNode, err := NewLeafNode(1, big.NewInt(1), []byte{1, 2, 3}).Serialize(testCtx)
	require.NoError(t, err)

	bucket := &fakeBucket{
		values: map[string][]byte{
			string([]byte{0}): emptyNode,
			string([]byte{1}): leafNode,
		},
	}

	return bucket
}

type fakeTx struct {
	kv.WritableTx

	bucket kv.Bucket
}

func (tx fakeTx) GetBucket(name []byte) kv.Bucket {
	return tx.bucket
}

func (tx fakeTx) GetBucketOrCreate(name []byte) (kv.Bucket, error) {
	return nil, nil
}

type fakeDB struct {
	kv.DB
	err error
}

func (db fakeDB) View(fn func(kv.ReadableTx) error) error {
	return fn(fakeTx{})
}

func (db fakeDB) Update(fn func(kv.WritableTx) error) error {
	if db.err != nil {
		return db.err
	}
	return fn(fakeTx{})
}

type fakeNode struct {
	TreeNode

	data []byte
	err  error
}

func (n fakeNode) Search(key *big.Int, path *Path, bucket kv.Bucket) ([]byte, error) {
	return nil, n.err
}

func (n fakeNode) Insert(key *big.Int, value []byte, b kv.Bucket) (TreeNode, error) {
	return n, n.err
}

func (n fakeNode) Delete(key *big.Int, b kv.Bucket) (TreeNode, error) {
	return nil, n.err
}

func (n fakeNode) Prepare(nonce []byte,
	prefix *big.Int, b kv.Bucket, fac crypto.HashFactory) ([]byte, error) {

	return n.data, n.err
}

func (n fakeNode) Visit(func(TreeNode) error) error {
	return n.err
}
