package mem

import (
	"math/big"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/internal/testing/fake"
	"golang.org/x/xerrors"
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

	next := node.Insert(prepareKey([]byte("ping")), []byte("pong"))
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
	require.Equal(t, "\x00\x01\x02\x00\x03\x00", string(calls.Get(0, 0).([]byte)))

	_, err = node.Prepare([]byte{1}, new(big.Int), fake.NewHashFactory(fake.NewBadHash()))
	require.EqualError(t, err, "empty node failed: fake error")
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

func TestInteriorNode_GetHash(t *testing.T) {
	node := NewInteriorNode(3)

	require.Empty(t, node.GetHash())

	node.hash = []byte{1, 2, 3}
	require.Equal(t, []byte{1, 2, 3}, node.GetHash())
}

func TestInteriorNode_GetType(t *testing.T) {
	node := NewInteriorNode(3)

	require.Equal(t, interiorNodeType, node.GetType())
}

func TestInteriorNode_Search(t *testing.T) {
	node := NewInteriorNode(0)
	node.left = NewLeafNode(1, big.NewInt(0).Bytes(), []byte("ping"))
	node.right = NewLeafNode(1, big.NewInt(1).Bytes(), []byte("pong"))

	path := newPath(nil)
	value := node.Search(big.NewInt(0), &path)
	require.Equal(t, "ping", string(value))
	require.Len(t, path.interiors, 1)

	path = newPath(nil)
	value = node.Search(big.NewInt(1), &path)
	require.Equal(t, "pong", string(value))
	require.Len(t, path.interiors, 1)
}

func TestInteriorNode_Insert(t *testing.T) {
	node := NewInteriorNode(0)

	next := node.Insert(big.NewInt(0), []byte("ping"))
	require.Same(t, node, next)

	next = node.Insert(big.NewInt(1), []byte("pong"))
	require.Same(t, node, next)
}

func TestInteriorNode_Delete(t *testing.T) {
	node := NewInteriorNode(0)
	node.left = NewLeafNode(1, big.NewInt(0).Bytes(), []byte("ping"))
	node.right = NewLeafNode(1, big.NewInt(1).Bytes(), []byte("pong"))

	next := node.Delete(big.NewInt(0))
	require.Same(t, node, next)

	next = node.Delete(big.NewInt(1))
	require.IsType(t, (*EmptyNode)(nil), next)
}

func TestInteriorNode_Prepare(t *testing.T) {
	node := NewInteriorNode(1)
	node.left = fakeNode{data: []byte{0xaa}}
	node.right = fakeNode{data: []byte{0xbb}}
	calls := &fake.Call{}

	_, err := node.Prepare([]byte{1}, big.NewInt(1), fake.NewHashFactory(&fake.Hash{Call: calls}))
	require.NoError(t, err)
	require.Equal(t, 1, calls.Len())
	require.Equal(t, "\xaa\xbb", string(calls.Get(0, 0).([]byte)))

	node.left = fakeNode{err: xerrors.New("bad node error")}
	_, err = node.Prepare([]byte{1}, big.NewInt(2), crypto.NewSha256Factory())
	require.EqualError(t, err, "bad node error")

	node.left = fakeNode{}
	node.right = fakeNode{err: xerrors.New("bad node error")}
	_, err = node.Prepare([]byte{1}, big.NewInt(2), crypto.NewSha256Factory())
	require.EqualError(t, err, "bad node error")

	node.right = fakeNode{}
	_, err = node.Prepare([]byte{1}, big.NewInt(2), fake.NewHashFactory(fake.NewBadHash()))
	require.EqualError(t, err, "interior node failed: fake error")
}

func TestInteriorNode_Visit(t *testing.T) {
	node := NewInteriorNode(0)

	counter := 0
	node.Visit(func(n TreeNode) {
		if counter == 0 {
			require.IsType(t, node, n)
		} else {
			require.IsType(t, (*EmptyNode)(nil), n)
		}
		counter++
	})

	require.Equal(t, 3, counter)
}

func TestInteriorNode_Clone(t *testing.T) {
	node := NewInteriorNode(0)

	clone := node.Clone()
	require.Equal(t, node, clone)
}

func TestLeafNode_GetHash(t *testing.T) {
	node := NewLeafNode(0, []byte("ping"), []byte("pong"))

	require.Empty(t, node.GetHash())

	_, err := node.Prepare([]byte{1}, big.NewInt(2), crypto.NewSha256Factory())
	require.NoError(t, err)
	require.Len(t, node.GetHash(), 32)
}

func TestLeafNode_GetType(t *testing.T) {
	node := NewLeafNode(0, []byte("ping"), []byte("pong"))

	require.Equal(t, leafNodeType, node.GetType())
}

func TestLeafNode_Search(t *testing.T) {
	node := NewLeafNode(0, []byte("ping"), []byte("pong"))
	path := newPath([]byte("ping"))

	value := node.Search(prepareKey([]byte("ping")), &path)
	require.Equal(t, []byte("pong"), value)
	require.Equal(t, node, path.leaf)

	value = node.Search(prepareKey([]byte("pong")), nil)
	require.Nil(t, value)
}

func TestLeafNode_Insert(t *testing.T) {
	node := NewLeafNode(0, []byte("ping"), []byte("pong"))

	next := node.Insert(prepareKey([]byte("ping")), []byte("abc"))
	require.Same(t, node, next)
	require.Equal(t, []byte("abc"), next.(*LeafNode).value)

	node = NewLeafNode(0, []byte{0}, []byte{0xaa})
	next = node.Insert(prepareKey([]byte{1}), []byte{0xbb})
	require.IsType(t, (*InteriorNode)(nil), next)
	require.IsType(t, (*LeafNode)(nil), next.(*InteriorNode).left)
	require.IsType(t, (*LeafNode)(nil), next.(*InteriorNode).right)

	node = NewLeafNode(0, []byte{1}, []byte{0xaa})
	next = node.Insert(prepareKey([]byte{0}), []byte{0xbb})
	require.IsType(t, (*InteriorNode)(nil), next)
	require.IsType(t, (*LeafNode)(nil), next.(*InteriorNode).left)
	require.IsType(t, (*LeafNode)(nil), next.(*InteriorNode).right)
}

func TestLeafNode_Delete(t *testing.T) {
	node := NewLeafNode(0, []byte("ping"), []byte("pong"))

	next := node.Delete(prepareKey([]byte("pong")))
	require.Same(t, node, next)

	next = node.Delete(prepareKey([]byte("ping")))
	require.IsType(t, (*EmptyNode)(nil), next)
}

func TestLeafNode_Prepare(t *testing.T) {
	node := NewLeafNode(3, []byte("ping"), []byte("pong"))
	calls := &fake.Call{}

	_, err := node.Prepare([]byte{1}, big.NewInt(2), fake.NewHashFactory(&fake.Hash{Call: calls}))
	require.NoError(t, err)
	require.Equal(t, 1, calls.Len())
	require.Equal(t, "\x02\x01\x03\x00\x02\x00pingpong", string(calls.Get(0, 0).([]byte)))

	_, err = node.Prepare(nil, big.NewInt(0), fake.NewHashFactory(fake.NewBadHash()))
	require.EqualError(t, err, "leaf node failed: fake error")
}

func TestLeafNode_Visit(t *testing.T) {
	node := NewLeafNode(3, []byte("ping"), []byte("pong"))

	counter := 0
	node.Visit(func(n TreeNode) {
		counter++
		require.IsType(t, node, n)
	})

	require.Equal(t, 1, counter)
}

func TestLeafNode_Clone(t *testing.T) {
	node := NewLeafNode(3, []byte("ping"), []byte("pong"))

	clone := node.Clone()
	require.Equal(t, node, clone)
}

// Utility functions -----------------------------------------------------------

type fakeNode struct {
	TreeNode

	data []byte
	err  error
}

func (n fakeNode) Prepare(nonce []byte, prefix *big.Int, fac crypto.HashFactory) ([]byte, error) {
	return n.data, n.err
}
