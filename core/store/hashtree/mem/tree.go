package mem

import (
	"encoding/binary"
	"math"
	"math/big"

	types "go.dedis.ch/dela/core/store/hashtree/mem/json"
	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// Nonce is the type of the tree nonce.
type Nonce [8]byte

const (
	// DepthLength is the length in bytes of the binary representation of the
	// depth.
	DepthLength = 2

	// MaxDepth is the maximum depth the tree should reach. It is equivalent to
	// the maximum key length in bytes.
	MaxDepth = 32
)

const (
	emptyNodeType byte = iota
	interiorNodeType
	leafNodeType
	diskNodeType
)

// TreeNode is the interface for the different types of nodes that a Merkle tree
// could have.
type TreeNode interface {
	serde.Message

	GetHash() []byte

	GetType() byte

	Search(key *big.Int, path *Path, bucket kv.Bucket) ([]byte, error)

	Insert(key *big.Int, value []byte, bucket kv.Bucket) (TreeNode, error)

	Delete(key *big.Int, bucket kv.Bucket) (TreeNode, error)

	Prepare(nonce []byte, prefix *big.Int, bucket kv.Bucket, fac crypto.HashFactory) ([]byte, error)

	Visit(func(TreeNode))

	Clone() TreeNode
}

// Tree is an implementation of a Merkle binary prefix tree. Due to the
// structure of the tree, any prefix of a longer prefix is overridden which
// means that the key should have the same length.
//
// Mutable operations on the tree don't update the hash root. It can be done
// after a batch of operations or a single one by using the Prepare function.
type Tree struct {
	nonce    Nonce
	maxDepth int
	root     TreeNode
	bucket   []byte
	db       kv.DB
}

// NewTree creates a new empty tree.
func NewTree(nonce Nonce, db kv.DB) *Tree {
	return &Tree{
		nonce:    nonce,
		maxDepth: MaxDepth,
		root:     NewEmptyNode(0),
		bucket:   []byte("hashtree"),
		db:       db,
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
// When path is defined, it will be filled so the interior nodes and the leaf
// node so that it can prove the inclusion or the absence of the key.
func (t *Tree) Search(key []byte, path *Path) ([]byte, error) {
	if len(key) > t.maxDepth {
		return nil, xerrors.Errorf("mismatch key length %d > %d", len(key), t.maxDepth)
	}

	var value []byte
	err := t.db.View(t.bucket, func(b kv.Bucket) error {
		var err error
		value, err = t.root.Search(makeKey(key), path, b)

		return err
	})

	if err != nil {
		return nil, err
	}

	if path != nil {
		path.root = t.root.GetHash()
	}

	return value, nil
}

// Insert inserts the key in the tree.
func (t *Tree) Insert(key, value []byte) error {
	if len(key) > t.maxDepth {
		return xerrors.Errorf("mismatch key length %d > %d", len(key), t.maxDepth)
	}

	err := t.db.Update(t.bucket, func(b kv.Bucket) error {
		var err error
		t.root, err = t.root.Insert(makeKey(key), value, b)

		return err
	})

	if err != nil {
		return err
	}

	return nil
}

// Delete removes a key from the tree.
func (t *Tree) Delete(key []byte) error {
	if len(key) > t.maxDepth {
		return xerrors.Errorf("mismatch key length %d > %d", len(key), t.maxDepth)
	}

	return t.db.Update(t.bucket, func(b kv.Bucket) error {
		var err error
		t.root, err = t.root.Delete(makeKey(key), b)

		return err
	})
}

// Update updates the hashes of the tree.
func (t *Tree) Update(fac crypto.HashFactory) error {
	prefix := new(big.Int)

	return t.db.Update(t.bucket, func(b kv.Bucket) error {
		_, err := t.root.Prepare(t.nonce[:], prefix, b, fac)
		if err != nil {
			return xerrors.Errorf("failed to prepare: %v", err)
		}

		return nil
	})
}

// Clone returns a deep copy of the tree.
func (t *Tree) Clone() *Tree {
	return &Tree{
		nonce:    t.nonce,
		maxDepth: t.maxDepth,
		root:     t.root.Clone(),
		bucket:   t.bucket,
		db:       t.db,
	}
}

// EmptyNode is leaf node with no value.
type EmptyNode struct {
	depth uint16
	hash  []byte
}

// NewEmptyNode creates a new empty node.
func NewEmptyNode(depth uint16) *EmptyNode {
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
func (n *EmptyNode) Search(key *big.Int, path *Path, b kv.Bucket) ([]byte, error) {
	if path != nil {
		path.value = nil
	}

	return nil, nil
}

// Insert implements mem.TreeNode. It replaces the empty node by a leaf node
// that contains the key and the value.
func (n *EmptyNode) Insert(key *big.Int, value []byte, b kv.Bucket) (TreeNode, error) {
	return NewLeafNode(n.depth, key, value), nil
}

// Delete implements mem.TreeNode. It ignores the delete as an empty node
// already means the key is missing.
func (n *EmptyNode) Delete(key *big.Int, b kv.Bucket) (TreeNode, error) {
	return n, nil
}

// Prepare implements mem.TreeNode. It updates the hash of the node and return
// the digest.
func (n *EmptyNode) Prepare(nonce []byte,
	prefix *big.Int, b kv.Bucket, fac crypto.HashFactory) ([]byte, error) {

	h := fac.New()

	data := make([]byte, 1+len(nonce)+bilen(prefix)+DepthLength)
	cursor := 1
	data[0] = emptyNodeType
	copy(data[cursor:], nonce)
	cursor += len(nonce)
	copy(data[cursor:], prefix.Bytes())
	cursor += bilen(prefix)
	copy(data[cursor:], int2buffer(n.depth))

	_, err := h.Write(data)
	if err != nil {
		return nil, xerrors.Errorf("empty node failed: %v", err)
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

func (n *EmptyNode) Serialize(ctx serde.Context) ([]byte, error) {
	return nil, nil
}

// InteriorNode is a node with two children.
type InteriorNode struct {
	hash  []byte
	depth uint16
	left  TreeNode
	right TreeNode
}

// NewInteriorNode creates a new interior node with two empty nodes as children.
func NewInteriorNode(depth uint16) *InteriorNode {
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
func (n *InteriorNode) Search(key *big.Int, path *Path, b kv.Bucket) ([]byte, error) {
	if key.Bit(int(n.depth)) == 0 {
		if path != nil {
			path.interiors = append(path.interiors, n.right.GetHash())
		}

		return n.left.Search(key, path, b)
	}

	if path != nil {
		path.interiors = append(path.interiors, n.left.GetHash())
	}

	return n.right.Search(key, path, b)
}

// Insert implements mem.TreeNode. It inserts the key/value pair to the right
// path.
func (n *InteriorNode) Insert(key *big.Int, value []byte, b kv.Bucket) (TreeNode, error) {
	var err error
	if key.Bit(int(n.depth)) == 0 {
		n.left, err = n.left.Insert(key, value, b)
	} else {
		n.right, err = n.right.Insert(key, value, b)
	}

	return n, err
}

// Delete implements mem.TreeNode. It deletes the leaf node associated to the
// key if it exists, otherwise nothin will change.
func (n *InteriorNode) Delete(key *big.Int, b kv.Bucket) (TreeNode, error) {
	var err error
	if key.Bit(int(n.depth)) == 0 {
		n.left, err = n.left.Delete(key, b)
	} else {
		n.right, err = n.right.Delete(key, b)
	}

	if err != nil {
		return nil, err
	}

	if n.left.GetType() == emptyNodeType && n.right.GetType() == emptyNodeType {
		// If an interior node points to two empty nodes, it is itself an empty
		// one.
		return NewEmptyNode(n.depth), nil
	}

	return n, nil
}

// Prepare implements mem.TreeNode. It updates the hash of the node and returns
// the digest.
func (n *InteriorNode) Prepare(nonce []byte,
	prefix *big.Int, b kv.Bucket, fac crypto.HashFactory) ([]byte, error) {

	h := fac.New()

	left, err := n.left.Prepare(nonce, new(big.Int).SetBit(prefix, int(n.depth), 0), b, fac)
	if err != nil {
		// No wrapping to prevent recursive calls to create huge error messages.
		return nil, err
	}

	right, err := n.right.Prepare(nonce, new(big.Int).SetBit(prefix, int(n.depth), 1), b, fac)
	if err != nil {
		// No wrapping to prevent recursive calls to create huge error messages.
		return nil, err
	}

	_, err = h.Write(append(left, right...))
	if err != nil {
		return nil, xerrors.Errorf("interior node failed: %v", err)
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

func (n *InteriorNode) Serialize(ctx serde.Context) ([]byte, error) {
	return nil, nil
}

// LeafNode is a leaf node with a key and a value.
type LeafNode struct {
	hash  []byte
	depth uint16
	key   *big.Int
	value []byte
}

// NewLeafNode creates a new leaf node.
func NewLeafNode(depth uint16, key *big.Int, value []byte) *LeafNode {
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
func (n *LeafNode) Search(key *big.Int, path *Path, b kv.Bucket) ([]byte, error) {
	if path != nil {
		path.value = n.value
	}

	if n.key.Cmp(key) == 0 {
		return n.value, nil
	}

	return nil, nil
}

// Insert implements mem.TreeNode. It replaces the leaf node by an interior node
// that contains both the current pair and the new one to insert.
func (n *LeafNode) Insert(key *big.Int, value []byte, b kv.Bucket) (TreeNode, error) {
	if n.key.Cmp(key) == 0 {
		n.value = value
		return n, nil
	}

	node := NewInteriorNode(n.depth)
	var err error

	// Both the leaf pair and the new one are inserted one after the other as
	// they could both end up in the same path, or on a different one.
	if n.key.Bit(int(n.depth)) == 0 {
		node.left, err = node.left.Insert(n.key, n.value, b)
	} else {
		node.right, err = node.right.Insert(n.key, n.value, b)
	}

	if err != nil {
		return nil, err
	}

	if key.Bit(int(n.depth)) == 0 {
		node.left, err = node.left.Insert(key, value, b)
	} else {
		node.right, err = node.right.Insert(key, value, b)
	}

	if err != nil {
		return nil, err
	}

	return node, nil
}

// Delete implements mem.TreeNode. It removes the leaf if the key matches.
func (n *LeafNode) Delete(key *big.Int, b kv.Bucket) (TreeNode, error) {
	if n.key.Cmp(key) == 0 {
		return NewEmptyNode(n.depth), nil
	}

	return n, nil
}

// Prepare implements mem.TreeNode. It updates the hash of the node and return
// the digest.
func (n *LeafNode) Prepare(nonce []byte,
	prefix *big.Int, b kv.Bucket, fac crypto.HashFactory) ([]byte, error) {

	h := fac.New()

	data := make([]byte, 1+len(nonce)+DepthLength+bilen(prefix)+bilen(n.key)+len(n.value))
	data[0] = leafNodeType
	cursor := 1
	copy(data[cursor:], nonce)
	cursor += len(nonce)
	copy(data[cursor:], int2buffer(n.depth))
	cursor += DepthLength
	copy(data[cursor:], prefix.Bytes())
	cursor += bilen(prefix)
	copy(data[cursor:], n.key.Bytes())
	cursor += bilen(n.key)
	copy(data[cursor:], n.value)

	_, err := h.Write(data)
	if err != nil {
		return nil, xerrors.Errorf("leaf node failed: %v", err)
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

func (n *LeafNode) Serialize(ctx serde.Context) ([]byte, error) {
	m := types.LeafNodeJSON{
		Digest: n.hash,
		Depth:  n.depth,
		Key:    n.key.Bytes(),
		Value:  n.value,
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, err
	}

	return data, nil
}

type NodeFactory struct{}

func (f NodeFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	m := types.LeafNodeJSON{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}

	key := new(big.Int)
	key.SetBytes(m.Key)

	node := &LeafNode{
		hash:  m.Digest,
		depth: m.Depth,
		key:   key,
		value: m.Value,
	}

	return node, nil
}

func int2buffer(depth uint16) []byte {
	buffer := make([]byte, 2)
	binary.LittleEndian.PutUint16(buffer, depth)

	return buffer
}

// makeKey is a helper to transform a key in bytes to its big number
// equivalence.
func makeKey(key []byte) *big.Int {
	bi := new(big.Int)
	bi.SetBytes(key)

	return bi
}

func bilen(n *big.Int) int {
	return int(math.Ceil(float64(n.BitLen()) / 8.0))
}
