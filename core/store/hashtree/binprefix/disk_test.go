package binprefix

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/serde/json"
)

var testCtx = json.NewContext()

func TestDiskNode_GetHash(t *testing.T) {
	node := &DiskNode{hash: []byte{0xaa}}

	require.Equal(t, []byte{0xaa}, node.GetHash())
}

func TestDiskNode_GetType(t *testing.T) {
	node := &DiskNode{}

	require.Equal(t, diskNodeType, node.GetType())
}

func TestDiskNode_Search(t *testing.T) {
	node := NewDiskNode(0, nil, testCtx, NodeFactory{})

	// Test if a leaf node can be loaded and searched for.
	leaf := NewLeafNode(0, big.NewInt(0), []byte("pong"))
	data, err := leaf.Serialize(testCtx)
	require.NoError(t, err)

	bucketKey := node.prepareKey(big.NewInt(0))

	value, err := node.Search(big.NewInt(0), nil, newFakeBucket(bucketKey, data))
	require.NoError(t, err)
	require.Equal(t, []byte("pong"), value)

	// Error if the node is not stored in the database.
	_, err = node.Search(big.NewInt(0), nil, &fakeBucket{})
	require.EqualError(t, err, "failed to load node: prefix 0 (depth 0) not in database")

	inter := NewInteriorNode(0, big.NewInt(0))
	data, err = inter.Serialize(testCtx)
	require.NoError(t, err)

	// Error deeper in the tree.
	_, err = node.Search(big.NewInt(0), nil, newFakeBucket(bucketKey, data))
	require.EqualError(t, err, "failed to load node: prefix 0 (depth 1) not in database")

	_, err = node.Search(big.NewInt(0), nil, nil)
	require.EqualError(t, err, "bucket is nil")
}

func TestDiskNode_Insert(t *testing.T) {
	node := NewDiskNode(0, nil, testCtx, NodeFactory{})

	empty := NewEmptyNode(0, big.NewInt(0))
	data, err := empty.Serialize(testCtx)
	require.NoError(t, err)

	bucket := newFakeBucket(node.prepareKey(big.NewInt(0)), data)

	next, err := node.Insert(big.NewInt(2), []byte("pong"), bucket)
	require.NoError(t, err)
	require.IsType(t, (*LeafNode)(nil), next)

	_, err = node.Insert(big.NewInt(2), nil, &fakeBucket{})
	require.EqualError(t, err, "failed to load node: prefix 10 (depth 0) not in database")

	inter := NewInteriorNode(0, big.NewInt(0))
	data, err = inter.Serialize(testCtx)
	require.NoError(t, err)

	bucket = newFakeBucket(node.prepareKey(big.NewInt(0)), data)

	_, err = node.Insert(big.NewInt(2), []byte("pong"), bucket)
	require.EqualError(t, err, "failed to load node: prefix 10 (depth 1) not in database")
}

func TestDiskNode_Delete(t *testing.T) {
	node := NewDiskNode(0, nil, testCtx, NodeFactory{})

	empty := NewEmptyNode(0, big.NewInt(0))
	data, err := empty.Serialize(testCtx)
	require.NoError(t, err)

	bucket := newFakeBucket(node.prepareKey(big.NewInt(0)), data)

	next, err := node.Delete(big.NewInt(2), bucket)
	require.NoError(t, err)
	require.IsType(t, (*EmptyNode)(nil), next)

	_, err = node.Delete(big.NewInt(2), &fakeBucket{})
	require.EqualError(t, err, "failed to load node: prefix 10 (depth 0) not in database")

	inter := NewInteriorNode(0, big.NewInt(0))
	data, err = inter.Serialize(testCtx)
	require.NoError(t, err)

	bucket = newFakeBucket(node.prepareKey(big.NewInt(0)), data)

	_, err = node.Delete(big.NewInt(2), bucket)
	require.EqualError(t, err, "failed to load node: prefix 10 (depth 1) not in database")

	_, err = node.Delete(big.NewInt(3), bucket)
	require.EqualError(t, err, "failed to load node: prefix 11 (depth 1) not in database")
}

func TestDiskNode_Prepare(t *testing.T) {
	node := NewDiskNode(0, nil, testCtx, NodeFactory{})

	empty := NewEmptyNode(0, big.NewInt(0))
	data, err := empty.Serialize(testCtx)
	require.NoError(t, err)

	bucket := newFakeBucket(node.prepareKey(big.NewInt(0)), data)

	hash, err := node.Prepare([]byte{1}, big.NewInt(0), bucket, crypto.NewSha256Factory())
	require.NoError(t, err)
	require.Len(t, hash, 32)
	require.Equal(t, hash, node.hash)

	node.hash = nil
	_, err = node.Prepare([]byte{}, big.NewInt(0), &fakeBucket{}, nil)
	require.EqualError(t, err, "failed to load node: prefix 0 (depth 0) not in database")

	bucket.errSet = fake.GetError()
	_, err = node.Prepare([]byte{}, big.NewInt(0), bucket, crypto.NewSha256Factory())
	require.EqualError(t, err, fake.Err("failed to store node: failed to set key"))

	inter := NewInteriorNode(0, big.NewInt(0))
	data, err = inter.Serialize(testCtx)
	require.NoError(t, err)

	bucket = newFakeBucket(node.prepareKey(big.NewInt(0)), data)

	_, err = node.Prepare([]byte{}, big.NewInt(0), bucket, crypto.NewSha256Factory())
	require.EqualError(t, err, "failed to load node: prefix 0 (depth 1) not in database")
}

func TestDiskNode_Visit(t *testing.T) {
	node := NewDiskNode(0, nil, testCtx, NodeFactory{})

	counter := 0
	node.Visit(func(n TreeNode) error {
		require.Same(t, node, n)
		counter++
		return nil
	})

	require.Equal(t, 1, counter)
}

func TestDiskNode_Clone(t *testing.T) {
	node := NewDiskNode(0, nil, testCtx, NodeFactory{})

	clone := node.Clone()
	require.Equal(t, node, clone)
}

func TestDiskNode_Serialize(t *testing.T) {
	node := NewDiskNode(0, nil, testCtx, NodeFactory{})

	_, err := node.Serialize(testCtx)
	require.Error(t, err)
}

func TestDiskNode_Load(t *testing.T) {
	node := NewDiskNode(0, nil, testCtx, NodeFactory{})

	empty := NewEmptyNode(0, big.NewInt(0))
	data, err := empty.Serialize(testCtx)
	require.NoError(t, err)

	bucket := newFakeBucket(node.prepareKey(big.NewInt(0)), data)

	n, err := node.load(big.NewInt(0), bucket)
	require.NoError(t, err)
	require.IsType(t, empty, n)

	node.factory = fake.NewBadMessageFactory()
	_, err = node.load(big.NewInt(0), bucket)
	require.EqualError(t, err, fake.Err("failed to deserialize"))

	node.factory = fake.MessageFactory{}
	_, err = node.load(big.NewInt(0), bucket)
	require.EqualError(t, err, "invalid node of type 'fake.Message'")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeBucket struct {
	kv.Bucket
	values  map[string][]byte
	errSet  error
	errScan error
}

func newFakeBucket(key, value []byte) *fakeBucket {
	return &fakeBucket{
		values: map[string][]byte{
			string(key): value,
		},
	}
}

func (b *fakeBucket) Get(key []byte) []byte {
	return b.values[string(key)]
}

func (b *fakeBucket) Set(key, value []byte) error {
	if b.values == nil {
		b.values = make(map[string][]byte)
	}

	b.values[string(key)] = value
	return b.errSet
}

func (b *fakeBucket) Delete(key []byte) error {
	return nil
}

func (b *fakeBucket) Scan(prefix []byte, fn func(k, v []byte) error) error {
	for key, value := range b.values {
		err := fn([]byte(key), value)
		if err != nil {
			return err
		}
	}

	return b.errScan
}
