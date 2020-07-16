package mem

import (
	"math/big"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestTree_Len(t *testing.T) {
	tree := NewTree(Nonce{})
	require.Equal(t, 0, tree.Len())

	tree.root = NewInteriorNode(0)
	require.Equal(t, 0, tree.Len())

	tree.root.(*InteriorNode).left = NewLeafNode(1, nil, nil)
	require.Equal(t, 1, tree.Len())

	tree.root.(*InteriorNode).right = NewLeafNode(1, nil, nil)
	require.Equal(t, 2, tree.Len())
}

func TestTree_Search(t *testing.T) {
	tree := NewTree(Nonce{})

	value, err := tree.Search(nil, nil)
	require.NoError(t, err)
	require.Nil(t, value)

	tree.root = NewLeafNode(0, []byte("A"), []byte("B"))
	value, err = tree.Search([]byte("A"), nil)
	require.NoError(t, err)
	require.Equal(t, []byte("B"), value)

	_, err = tree.Search(make([]byte, MaxDepth+1), nil)
	require.EqualError(t, err, "mismatch key length 33 > 32")
}

func TestTree_Insert(t *testing.T) {
	tree := NewTree(Nonce{})

	err := tree.Insert([]byte("ping"), []byte("pong"))
	require.NoError(t, err)
	require.Equal(t, 1, tree.Len())

	err = tree.Insert(make([]byte, MaxDepth+1), nil)
	require.EqualError(t, err, "mismatch key length 33 > 32")
}

func TestTree_Delete(t *testing.T) {
	tree := NewTree(Nonce{})

	err := tree.Delete(nil)
	require.NoError(t, err)

	err = tree.Delete([]byte("ping"))
	require.NoError(t, err)

	tree.root = NewLeafNode(0, []byte("ping"), []byte("pong"))
	err = tree.Delete([]byte("ping"))
	require.NoError(t, err)
	require.IsType(t, (*EmptyNode)(nil), tree.root)

	err = tree.Delete(make([]byte, MaxDepth+1))
	require.EqualError(t, err, "mismatch key length 33 > 32")
}

func TestTree_Clone(t *testing.T) {
	tree := NewTree(Nonce{})

	f := func(key [8]byte, value []byte) bool {
		return tree.Insert(key[:], value) == nil
	}

	err := quick.Check(f, nil)
	require.NoError(t, err)

	clone := tree.Clone()
	require.Equal(t, tree.Len(), clone.Len())

	require.NoError(t, tree.Update(crypto.NewSha256Factory()))
	require.NoError(t, clone.Update(crypto.NewSha256Factory()))
	require.Equal(t, tree.root.GetHash(), clone.root.GetHash())
}

func TestEmptyNode_GetHash(t *testing.T) {
	node := NewEmptyNode(0)
	require.Empty(t, node.GetHash())

	node.hash = []byte("ping")
	require.Equal(t, []byte("ping"), node.GetHash())
}

func TestEmptyNode_GetType(t *testing.T) {
	node := NewEmptyNode(0)
	require.Equal(t, emptyNodeType, node.GetType())
}

func TestEmptyNode_Search(t *testing.T) {
	node := NewEmptyNode(0)
	path := newPath(nil)

	value := node.Search(new(big.Int), &path)
	require.Nil(t, value)
	require.Same(t, node, path.leaf)
}

func TestEmptyNode_Insert(t *testing.T) {
	node := NewEmptyNode(0)

	next := node.Insert([]byte("ping"), []byte("pong"), nil)
	require.IsType(t, (*LeafNode)(nil), next)
}

func TestEmptyNode_Delete(t *testing.T) {
	node := NewEmptyNode(0)

	require.Same(t, node, node.Delete(nil))
}

func TestEmptyNode_Prepare(t *testing.T) {
	node := NewEmptyNode(3)

	fac := crypto.NewSha256Factory()

	hash, err := node.Prepare([]byte("nonce"), new(big.Int), fac)
	require.NoError(t, err)
	require.Equal(t, hash, node.hash)

	calls := &fake.Call{}
	_, err = node.Prepare([]byte{1}, new(big.Int).SetBytes([]byte{2}), fake.NewHashFactory(&fake.Hash{Call: calls}))
	require.NoError(t, err)
	require.Equal(t, 1, calls.Len())
	require.Equal(t, "\x00\x01\x02\x00\x03\x00\x00\x00\x00\x00\x00\x00", string(calls.Get(0, 0).([]byte)))

	_, err = node.Prepare([]byte{1}, new(big.Int), fake.NewHashFactory(fake.NewBadHash()))
	require.EqualError(t, err, "failed to write: fake error")
}

func TestEmptyNode_Visit(t *testing.T) {
	node := NewEmptyNode(3)
	count := 0

	node.Visit(func(tn TreeNode) {
		require.Same(t, node, tn)
		count++
	})

	require.Equal(t, 1, count)
}

func TestEmptyNode_Clone(t *testing.T) {
	node := NewEmptyNode(3)

	clone := node.Clone()
	require.Equal(t, node, clone)
}
