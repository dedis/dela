// Package mem defines a merkle binary prefix tree that is implementing the
// CONIKS paper.
//
// https://www.usenix.org/system/files/conference/usenixsecurity15/sec15-paper-melara.pdf
package mem

import (
	"encoding/binary"
	"math/big"

	"go.dedis.ch/dela/crypto"
	"golang.org/x/xerrors"
)

const (
	// NonceLength is the length in bytes of the tree nonce.
	NonceLength = 8
)

const (
	emptyNodeType byte = iota
	interiorNodeType
	leafNodeType
)

// Tree is an implementation of a Merkle binary prefix tree. Due to the
// structure of the tree, any prefix of an index is overriden which means that
// the key should have the same length.
type Tree struct {
	nonce    [NonceLength]byte
	maxDepth int
	root     TreeNode
}

// NewTree creates a new empty tree.
func NewTree(maxDepth int) *Tree {
	return &Tree{
		maxDepth: maxDepth,
		root:     NewEmptyNode(0),
	}
}

// Len returns the number of leaves in the tree.
func (t *Tree) Len() int {
	counter := 0

	t.root.Visit(func(n TreeNode) {
		if n.GetType() == leafNodeType {
			counter++
		}
	})

	return counter
}

// Search returns the value associated to the key if it exists, otherwise nil.
func (t *Tree) Search(key []byte, path *Path) ([]byte, error) {
	if len(key) > t.maxDepth {
		return nil, xerrors.Errorf("mismatch key length %d > %d", len(key), t.maxDepth)
	}

	// Build the big int representation of the key that is used for bitwise
	// operations.
	bi := new(big.Int)
	bi.SetBytes(key)

	value := t.root.Search(bi, path)

	return value, nil
}

// Insert inserts the key in the tree.
func (t *Tree) Insert(key []byte, value []byte) error {
	if len(key) > t.maxDepth {
		return xerrors.Errorf("mismatch key length %d > %d", len(key), t.maxDepth)
	}

	// Build the big int representation of the key that is used for bitwise
	// operations.
	bi := new(big.Int)
	bi.SetBytes(key)

	t.root = t.root.Insert(key, value, bi)

	return nil
}

// Delete removes a key from the tree.
func (t *Tree) Delete(key []byte) error {
	if len(key) > t.maxDepth {
		return xerrors.Errorf("mismatch key length %d > %d", len(key), t.maxDepth)
	}

	bi := new(big.Int)
	bi.SetBytes(key)

	t.root = t.root.Delete(bi)

	return nil
}

// Update updates the hashes of the tree.
func (t *Tree) Update() error {
	prefix := new(big.Int)

	_, err := t.root.Prepare(t.nonce[:], prefix, crypto.NewSha256Factory())
	if err != nil {
		return xerrors.Errorf("failed to prepare: %v", err)
	}

	return nil
}

// Clone returns a deep copy of the tree.
func (t *Tree) Clone() *Tree {
	return &Tree{
		nonce:    t.nonce,
		maxDepth: t.maxDepth,
		root:     t.root.Clone(),
	}
}

// TreeNode is the interface for the different types of nodes that a Merkle tree
// could have.
type TreeNode interface {
	GetHash() []byte

	GetType() byte

	Search(key *big.Int, path *Path) []byte

	Insert(key, value []byte, bi *big.Int) TreeNode

	Delete(key *big.Int) TreeNode

	Prepare(nonce []byte, prefix *big.Int, fac crypto.HashFactory) ([]byte, error)

	Visit(func(TreeNode))

	Clone() TreeNode
}

// EmptyNode is leaf node with no value.
type EmptyNode struct {
	depth int
	hash  []byte
}

// NewEmptyNode creates a new empty node.
func NewEmptyNode(depth int) *EmptyNode {
	return &EmptyNode{
		depth: depth,
	}
}

// GetHash implements mem.TreeNode. It returns the hash of the node.
func (n *EmptyNode) GetHash() []byte {
	return append([]byte{}, n.hash...)
}

// GetType implements mem.TreeNode. It returns the empty node type.
func (n *EmptyNode) GetType() byte {
	return emptyNodeType
}

// Search implements mem.TreeNode. It always return a empty value.
func (n *EmptyNode) Search(key *big.Int, path *Path) []byte {
	if path != nil {
		path.leaf = n
	}

	return nil
}

// Insert implements mem.TreeNode. It replaces the empty node by a leaf node
// that contains the key and the value.
func (n *EmptyNode) Insert(key, value []byte, bi *big.Int) TreeNode {
	return NewLeafNode(n.depth, key, value)
}

// Delete implements mem.TreeNode. It ignores the delete as an empty node
// already means the key is missing.
func (n *EmptyNode) Delete(key *big.Int) TreeNode {
	return n
}

// Prepare implements mem.TreeNode. It updates the hash of the node and return
// the digest.
func (n *EmptyNode) Prepare(nonce []byte, prefix *big.Int, fac crypto.HashFactory) ([]byte, error) {
	h := fac.New()

	data := make([]byte, 1+len(nonce)+prefix.BitLen()+8)
	cursor := 1
	data[0] = emptyNodeType
	copy(data[cursor:], nonce)
	cursor += len(nonce)
	copy(data[cursor:], prefix.Bytes())
	cursor += prefix.BitLen()
	copy(data[cursor:], int2buffer(n.depth))

	_, err := h.Write(data)
	if err != nil {
		return nil, xerrors.Errorf("failed to write: %v", err)
	}

	n.hash = h.Sum(nil)

	return n.GetHash(), nil
}

// Visit implements mem.TreeNode. It executes the callback with the node.
func (n *EmptyNode) Visit(fn func(TreeNode)) {
	fn(n)
}

// Clone implements mem.TreeNode. It returns a deep copy of the empty node.
func (n *EmptyNode) Clone() TreeNode {
	return NewEmptyNode(n.depth)
}

// InteriorNode is a node with two children.
type InteriorNode struct {
	hash  []byte
	depth int
	left  TreeNode
	right TreeNode
}

// NewInteriorNode creates a new interior node with two empty nodes as children.
func NewInteriorNode(depth int) *InteriorNode {
	return &InteriorNode{
		depth: depth,
		left:  NewEmptyNode(depth + 1),
		right: NewEmptyNode(depth + 1),
	}
}

// GetHash implements mem.TreeNode. It returns the hash of the node.
func (n *InteriorNode) GetHash() []byte {
	return append([]byte{}, n.hash...)
}

// GetType implements mem.TreeNode. It returns the interior node type.
func (n *InteriorNode) GetType() byte {
	return interiorNodeType
}

// Search implements mem.TreeNode. It recursively search for the value in the
// correct child.
func (n *InteriorNode) Search(key *big.Int, path *Path) []byte {
	if path != nil {
		path.interiors = append(path.interiors, n.hash)
	}

	if key.Bit(n.depth) == 0 {
		return n.left.Search(key, path)
	}

	return n.right.Search(key, path)
}

// Insert implements mem.TreeNode. It inserts the key/value pair to the right
// path.
func (n *InteriorNode) Insert(key, value []byte, bi *big.Int) TreeNode {
	if bi.Bit(n.depth) == 0 {
		n.left = n.left.Insert(key, value, bi)
	} else {
		n.right = n.right.Insert(key, value, bi)
	}

	return n
}

// Delete implements mem.TreeNode. It deletes the key from the right path.
func (n *InteriorNode) Delete(key *big.Int) TreeNode {
	if key.Bit(n.depth) == 0 {
		n.left = n.left.Delete(key)
	} else {
		n.right = n.right.Delete(key)
	}

	if n.left.GetType() == emptyNodeType && n.right.GetType() == emptyNodeType {
		// If an interior node points to two empty nodes, it is itself an empty
		// one.
		return NewEmptyNode(n.depth)
	}

	return n
}

// Prepare implements mem.TreeNode. It updates the hash of the node and return
// the digest.
func (n *InteriorNode) Prepare(nonce []byte, prefix *big.Int, fac crypto.HashFactory) ([]byte, error) {
	h := fac.New()

	left, err := n.left.Prepare(nonce, new(big.Int).SetBit(prefix, n.depth, 0), fac)
	if err != nil {
		return nil, err
	}

	right, err := n.right.Prepare(nonce, new(big.Int).SetBit(prefix, n.depth, 1), fac)
	if err != nil {
		return nil, err
	}

	_, err = h.Write(append(left, right...))
	if err != nil {
		return nil, xerrors.Errorf("failed to write: %v", err)
	}

	n.hash = h.Sum(nil)

	return n.GetHash(), nil
}

// Visit implements mem.TreeNode. It executes the callback with the node and
// recursively with the children.
func (n *InteriorNode) Visit(fn func(TreeNode)) {
	fn(n)
	n.left.Visit(fn)
	n.right.Visit(fn)
}

// Clone implements mem.TreeNode. It returns a deep copy of the interior node.
func (n *InteriorNode) Clone() TreeNode {
	return &InteriorNode{
		depth: n.depth,
		left:  n.left.Clone(),
		right: n.right.Clone(),
	}
}

// LeafNode is a leaf node with a key and a value.
type LeafNode struct {
	hash  []byte
	depth int
	key   []byte
	value []byte
}

// NewLeafNode creates a new leaf node.
func NewLeafNode(depth int, key, value []byte) *LeafNode {
	return &LeafNode{
		depth: depth,
		key:   key,
		value: value,
	}
}

// GetHash implements mem.TreeNode. It returns the hash of the node.
func (n *LeafNode) GetHash() []byte {
	return append([]byte{}, n.hash...)
}

// GetType implements mem.TreeNode. It returns the leaf node type.
func (n *LeafNode) GetType() byte {
	return leafNodeType
}

// Search implements mem.TreeNode. It returns the value if the key matches.
func (n *LeafNode) Search(key *big.Int, path *Path) []byte {
	if path != nil {
		path.leaf = n
	}

	curr := new(big.Int)
	curr.SetBytes(n.key)

	if curr.Cmp(key) == 0 {
		return n.value
	}

	return nil
}

// Insert implements mem.TreeNode. It replaces the leaf node by an interior node
// that contains both the current pair and the new one to insert.
func (n *LeafNode) Insert(key, value []byte, bi *big.Int) TreeNode {
	curr := new(big.Int)
	curr.SetBytes(n.key)

	if curr.Cmp(bi) == 0 {
		n.value = value
		return n
	}

	node := NewInteriorNode(n.depth)

	if curr.Bit(n.depth) == 0 {
		node.left = node.left.Insert(n.key, n.value, curr)
	} else {
		node.right = node.right.Insert(n.key, n.value, curr)
	}

	if bi.Bit(n.depth) == 0 {
		node.left = node.left.Insert(key, value, bi)
	} else {
		node.right = node.right.Insert(key, value, bi)
	}

	return node
}

// Delete implements mem.TreeNode. It removes the leaf if the key matches.
func (n *LeafNode) Delete(key *big.Int) TreeNode {
	curr := new(big.Int)
	curr.SetBytes(n.key)

	if curr.Cmp(key) == 0 {
		return NewEmptyNode(n.depth)
	}

	return n
}

// Prepare implements mem.TreeNode. It updates the hash of the node and return
// the digest.
func (n *LeafNode) Prepare(nonce []byte, prefix *big.Int, fac crypto.HashFactory) ([]byte, error) {
	h := fac.New()

	data := make([]byte, 1+len(nonce)+n.depth+8+len(n.key)+len(n.value))
	data[0] = leafNodeType
	cursor := 1
	copy(data[cursor:], nonce)
	cursor += len(nonce)
	copy(data[cursor:], int2buffer(n.depth))
	cursor += 8
	copy(data[cursor:], prefix.Bytes())
	cursor += n.depth
	copy(data[cursor:], n.key)
	cursor += len(n.key)
	copy(data[cursor:], n.value)

	_, err := h.Write(data)
	if err != nil {
		return nil, xerrors.Errorf("failed to write: %v", err)
	}

	n.hash = h.Sum(nil)

	return n.GetHash(), nil
}

// Visit implements mem.TreeNode. It executes the callback with the node.
func (n *LeafNode) Visit(fn func(TreeNode)) {
	fn(n)
}

// Clone implements mem.TreeNode. It returns a copy of the leaf node.
func (n *LeafNode) Clone() TreeNode {
	return NewLeafNode(n.depth, n.key, n.value)
}

func int2buffer(depth int) []byte {
	buffer := make([]byte, 8)
	binary.LittleEndian.PutUint64(buffer, uint64(depth))

	return buffer
}
