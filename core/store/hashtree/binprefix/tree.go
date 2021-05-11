//
// Documentation Last Review: 08.10.2020
//

package binprefix

import (
	"encoding/binary"
	"math"
	"math/big"

	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
)

func init() {
	nodeFormats.Register(serde.FormatJSON, nodeFormat{})
}

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

var nodeFormats = registry.NewSimpleRegistry()

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

	Visit(func(node TreeNode) error) error

	Clone() TreeNode
}

// Tree is an implementation of a Merkle binary prefix tree. Due to the
// structure of the tree, any prefix of a longer prefix is overridden which
// means that the key should have the same length.
//
// Mutable operations on the tree don't update the hash root. It can be done
// after a batch of operations or a single one by using the CalculateRoot
// function.
type Tree struct {
	nonce    Nonce
	maxDepth int
	memDepth int
	root     TreeNode
	context  serde.Context
	factory  serde.Factory
}

// NewTree creates a new empty tree.
func NewTree(nonce Nonce) *Tree {
	return &Tree{
		nonce:    nonce,
		maxDepth: MaxDepth,
		memDepth: MaxDepth * 8,
		root:     NewEmptyNode(0, big.NewInt(0)),
		factory:  NodeFactory{},
		context:  json.NewContext(),
	}
}

// Len returns the number of leaves in the tree.
func (t *Tree) Len() int {
	counter := 0

	t.root.Visit(func(n TreeNode) error {
		if n.GetType() == leafNodeType {
			counter++
		}

		return nil
	})

	return counter
}

// FillFromBucket scans the bucket for leafs to insert them in the tree. It will
// then persist the tree to restore the memory depth limit.
func (t *Tree) FillFromBucket(bucket kv.Bucket) error {
	if bucket == nil {
		return nil
	}

	t.root = NewInteriorNode(0, big.NewInt(0))

	err := bucket.Scan([]byte{}, func(key, value []byte) error {
		msg, err := t.factory.Deserialize(t.context, value)
		if err != nil {
			return xerrors.Errorf("tree node malformed: %v", err)
		}

		var diskNode *DiskNode
		var prefix *big.Int

		switch node := msg.(type) {
		case *InteriorNode:
			diskNode = NewDiskNode(node.depth, node.hash, t.context, t.factory)
			prefix = node.prefix
		case *EmptyNode:
			diskNode = NewDiskNode(node.depth, node.hash, t.context, t.factory)
			prefix = node.prefix
		case *LeafNode:
			diskNode = NewDiskNode(node.depth, node.hash, t.context, t.factory)
			prefix = node.key
		}

		t.restore(t.root, prefix, diskNode)

		return nil
	})

	if err != nil {
		return xerrors.Errorf("while scanning: %v", err)
	}

	return nil
}

// Restore is a recursive function that will append the node to the tree by
// recreating the interior nodes from the root to its natural position defined
// by the prefix.
func (t *Tree) restore(curr TreeNode, prefix *big.Int, node *DiskNode) TreeNode {
	var interior *InteriorNode

	switch n := curr.(type) {
	case *InteriorNode:
		interior = n
	case *DiskNode:
		return curr
	case *EmptyNode:
		if node.depth == n.depth {
			return node
		}

		interior = NewInteriorNode(n.depth, n.prefix)
	}

	if prefix.Bit(int(interior.depth)) == 0 {
		interior.left = t.restore(interior.left, prefix, node)
	} else {
		interior.right = t.restore(interior.right, prefix, node)
	}

	return interior
}

// Search returns the value associated to the key if it exists, otherwise nil.
// When path is defined, it will be filled with the interior nodes and the leaf
// node so that it can prove the inclusion or the absence of the key.
func (t *Tree) Search(key []byte, path *Path, b kv.Bucket) ([]byte, error) {
	if len(key) > t.maxDepth {
		return nil, xerrors.Errorf("mismatch key length %d > %d", len(key), t.maxDepth)
	}

	value, err := t.root.Search(makeKey(key), path, b)
	if err != nil {
		return nil, xerrors.Errorf("failed to search: %v", err)
	}

	if path != nil {
		path.root = t.root.GetHash()
	}

	return value, nil
}

// Insert inserts the key in the tree.
func (t *Tree) Insert(key, value []byte, b kv.Bucket) error {
	if len(key) > t.maxDepth {
		return xerrors.Errorf("mismatch key length %d > %d", len(key), t.maxDepth)
	}

	var err error
	t.root, err = t.root.Insert(makeKey(key), value, b)
	if err != nil {
		return xerrors.Errorf("failed to insert: %v", err)
	}

	return nil
}

// Delete removes a key from the tree.
func (t *Tree) Delete(key []byte, b kv.Bucket) error {
	if len(key) > t.maxDepth {
		return xerrors.Errorf("mismatch key length %d > %d", len(key), t.maxDepth)
	}

	var err error
	t.root, err = t.root.Delete(makeKey(key), b)
	if err != nil {
		return xerrors.Errorf("failed to delete: %v", err)
	}

	return nil
}

// CalculateRoot updates the hashes of the tree.
func (t *Tree) CalculateRoot(fac crypto.HashFactory, b kv.Bucket) error {
	prefix := new(big.Int)

	_, err := t.root.Prepare(t.nonce[:], prefix, b, fac)
	if err != nil {
		return xerrors.Errorf("failed to prepare: %v", err)
	}

	return nil
}

// Persist visits the whole tree and stores the leaf node in the database and
// replaces the node with disk nodes. Depending of the parameter, it also stores
// intermediate nodes on the disk.
func (t *Tree) Persist(b kv.Bucket) error {
	return t.root.Visit(func(n TreeNode) error {
		switch node := n.(type) {
		case *InteriorNode:
			if int(node.depth) > t.memDepth {
				return t.toDisk(node.depth, node.prefix, node, b, false, true)
			}

			_, ok := node.left.(*LeafNode)
			if ok || int(node.depth) == t.memDepth {
				node.left = NewDiskNode(node.depth+1, node.left.GetHash(), t.context, t.factory)
			}

			_, ok = node.right.(*LeafNode)
			if ok || int(node.depth) == t.memDepth {
				node.right = NewDiskNode(node.depth+1, node.right.GetHash(), t.context, t.factory)
			}
		case *LeafNode:
			return t.toDisk(node.depth, node.key, node, b, true, true)
		case *EmptyNode:
			shouldStore := int(node.depth) >= t.memDepth

			return t.toDisk(node.depth, node.prefix, node, b, true, shouldStore)
		}

		return nil
	})
}

func (t *Tree) toDisk(depth uint16, prefix *big.Int, node TreeNode, b kv.Bucket, clean, store bool) error {
	disknode := NewDiskNode(depth, nil, t.context, t.factory)

	if clean {
		err := disknode.cleanSubtree(depth, prefix, b)
		if err != nil {
			return xerrors.Errorf("failed to clean subtree: %v", err)
		}
	}

	if store {
		err := disknode.store(prefix, node, b)
		if err != nil {
			return xerrors.Errorf("failed to store node: %v", err)
		}
	}

	return nil
}

// Clone returns a deep copy of the tree.
func (t *Tree) Clone() *Tree {
	return &Tree{
		nonce:    t.nonce,
		maxDepth: t.maxDepth,
		memDepth: t.memDepth,
		root:     t.root.Clone(),
		context:  t.context,
		factory:  t.factory,
	}
}

// EmptyNode is leaf node with no value.
//
// - implements binprefix.TreeNode
type EmptyNode struct {
	depth  uint16
	prefix *big.Int
	hash   []byte
}

// NewEmptyNode creates a new empty node.
func NewEmptyNode(depth uint16, key *big.Int) *EmptyNode {
	return NewEmptyNodeWithDigest(depth, key, nil)
}

// NewEmptyNodeWithDigest creates a new empty node with its digest.
func NewEmptyNodeWithDigest(depth uint16, key *big.Int, hash []byte) *EmptyNode {
	return &EmptyNode{
		depth:  depth,
		prefix: key,
		hash:   hash,
	}
}

// GetHash implements binprefix.TreeNode. It returns the hash of the node.
func (n *EmptyNode) GetHash() []byte {
	return append([]byte{}, n.hash...)
}

// GetType implements binprefix.TreeNode. It returns the empty node type.
func (n *EmptyNode) GetType() byte {
	return emptyNodeType
}

// GetDepth returns the depth of the node.
func (n *EmptyNode) GetDepth() uint16 {
	return n.depth
}

// GetPrefix returns the prefix of the node.
func (n *EmptyNode) GetPrefix() *big.Int {
	return n.prefix
}

// Search implements binprefix.TreeNode. It always return a empty value.
func (n *EmptyNode) Search(key *big.Int, path *Path, b kv.Bucket) ([]byte, error) {
	if path != nil {
		path.value = nil
	}

	return nil, nil
}

// Insert implements binprefix.TreeNode. It replaces the empty node by a leaf
// node that contains the key and the value.
func (n *EmptyNode) Insert(key *big.Int, value []byte, b kv.Bucket) (TreeNode, error) {
	return NewLeafNode(n.depth, key, value), nil
}

// Delete implements binprefix.TreeNode. It ignores the delete as an empty node
// already means the key is missing.
func (n *EmptyNode) Delete(key *big.Int, b kv.Bucket) (TreeNode, error) {
	return n, nil
}

// Prepare implements binprefix.TreeNode. It updates the hash of the node if not
// already set and returns the digest.
func (n *EmptyNode) Prepare(nonce []byte,
	prefix *big.Int, b kv.Bucket, fac crypto.HashFactory) ([]byte, error) {

	if len(n.hash) > 0 {
		// Hash is already calculated so we can skip and return.
		return n.hash, nil
	}

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

// Visit implements binprefix.TreeNode. It executes the callback with the node.
func (n *EmptyNode) Visit(fn func(node TreeNode) error) error {
	err := fn(n)
	if err != nil {
		return xerrors.Errorf("visiting empty: %v", err)
	}

	return nil
}

// Clone implements binprefix.TreeNode. It returns a deep copy of the empty
// node.
func (n *EmptyNode) Clone() TreeNode {
	return NewEmptyNode(n.depth, n.prefix)
}

// Serialize implements serde.Message. It returns the JSON data of the empty
// node.
func (n *EmptyNode) Serialize(ctx serde.Context) ([]byte, error) {
	format := nodeFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, n)
	if err != nil {
		return nil, xerrors.Errorf("failed to encode empty node: %v", err)
	}

	return data, nil
}

// InteriorNode is a node with two children.
//
// - implements binprefix.TreeNode
type InteriorNode struct {
	hash   []byte
	depth  uint16
	prefix *big.Int
	left   TreeNode
	right  TreeNode
}

// NewInteriorNode creates a new interior node with two empty nodes as children.
func NewInteriorNode(depth uint16, prefix *big.Int) *InteriorNode {
	return NewInteriorNodeWithChildren(
		depth,
		prefix,
		nil,
		NewEmptyNode(depth+1, new(big.Int).SetBit(prefix, int(depth), 0)),
		NewEmptyNode(depth+1, new(big.Int).SetBit(prefix, int(depth), 1)),
	)
}

// NewInteriorNodeWithChildren creates a new interior node with the two given
// children.
func NewInteriorNodeWithChildren(depth uint16, prefix *big.Int, hash []byte, left, right TreeNode) *InteriorNode {
	return &InteriorNode{
		depth:  depth,
		prefix: prefix,
		hash:   hash,
		left:   left,
		right:  right,
	}
}

// GetHash implements binprefix.TreeNode. It returns the hash of the node.
func (n *InteriorNode) GetHash() []byte {
	return append([]byte{}, n.hash...)
}

// GetType implements binprefix.TreeNode. It returns the interior node type.
func (n *InteriorNode) GetType() byte {
	return interiorNodeType
}

// GetDepth returns the depth of the node.
func (n *InteriorNode) GetDepth() uint16 {
	return n.depth
}

// GetPrefix returns the prefix of the node.
func (n *InteriorNode) GetPrefix() *big.Int {
	return n.prefix
}

// Search implements binprefix.TreeNode. It recursively search for the value in
// the correct child.
func (n *InteriorNode) Search(key *big.Int, path *Path, b kv.Bucket) ([]byte, error) {
	if key.Bit(int(n.depth)) == 0 {
		n.right, _ = n.load(n.right, key, 1, b)

		if path != nil {
			path.interiors = append(path.interiors, n.right.GetHash())
		}

		return n.left.Search(key, path, b)
	}

	n.left, _ = n.load(n.left, key, 0, b)

	if path != nil {
		path.interiors = append(path.interiors, n.left.GetHash())
	}

	return n.right.Search(key, path, b)
}

// Insert implements binprefix.TreeNode. It inserts the key/value pair to the
// right path.
func (n *InteriorNode) Insert(key *big.Int, value []byte, b kv.Bucket) (TreeNode, error) {
	var err error
	if key.Bit(int(n.depth)) == 0 {
		n.left, err = n.left.Insert(key, value, b)
	} else {
		n.right, err = n.right.Insert(key, value, b)
	}

	// Reset the hash as the subtree will change and thus invalidate this
	// current value.
	n.hash = nil

	return n, err
}

// Delete implements binprefix.TreeNode. It deletes the leaf node associated to
// the key if it exists, otherwise nothing will change.
func (n *InteriorNode) Delete(key *big.Int, b kv.Bucket) (TreeNode, error) {
	var err error

	// Depending on the path to follow, it will delete the key from the correct
	// path and it will load the first node of the opposite path if it is a disk
	// node so that the type can be compared. Errors are not wrapper to prevent
	// very long error message.
	if key.Bit(int(n.depth)) == 0 {
		n.left, err = n.left.Delete(key, b)
		if err != nil {
			return nil, err
		}

		n.right, err = n.load(n.right, key, 1, b)
		if err != nil {
			return nil, err
		}
	} else {
		n.right, err = n.right.Delete(key, b)
		if err != nil {
			return nil, err
		}

		n.left, err = n.load(n.left, key, 0, b)
		if err != nil {
			return nil, err
		}
	}

	if n.left.GetType() == emptyNodeType && n.right.GetType() == emptyNodeType {
		// If an interior node points to two empty nodes, it is itself an empty
		// one.
		return NewEmptyNode(n.depth, n.prefix), nil
	}

	return n, nil
}

func (n *InteriorNode) load(node TreeNode, key *big.Int, bit uint, b kv.Bucket) (TreeNode, error) {
	diskn, ok := node.(*DiskNode)
	if !ok {
		return node, nil
	}

	node, err := diskn.load(new(big.Int).SetBit(key, int(n.depth), bit), b)
	if err != nil {
		return nil, xerrors.Errorf("failed to load node: %v", err)
	}

	return node, nil
}

// Prepare implements binprefix.TreeNode. It updates the hash of the node if not
// already set and returns the digest.
func (n *InteriorNode) Prepare(nonce []byte,
	prefix *big.Int, b kv.Bucket, fac crypto.HashFactory) ([]byte, error) {

	if len(n.hash) > 0 {
		// Hash is already calculated so we can skip and return.
		return n.hash, nil
	}

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

// Visit implements binprefix.TreeNode. It executes the callback with the node
// and recursively with the children.
func (n *InteriorNode) Visit(fn func(TreeNode) error) error {
	err := n.left.Visit(fn)
	if err != nil {
		// No wrapping to prevent long error message from recursive calls.
		return err
	}

	err = n.right.Visit(fn)
	if err != nil {
		// No wrapping to prevent long error message from recursive calls.
		return err
	}

	err = fn(n)
	if err != nil {
		return xerrors.Errorf("visiting interior: %v", err)
	}

	return nil
}

// Clone implements binprefix.TreeNode. It returns a deep copy of the interior
// node.
func (n *InteriorNode) Clone() TreeNode {
	return &InteriorNode{
		depth:  n.depth,
		prefix: n.prefix,
		left:   n.left.Clone(),
		right:  n.right.Clone(),
	}
}

// Serialize implements serde.Message. It returns the JSON data of the interior
// node.
func (n *InteriorNode) Serialize(ctx serde.Context) ([]byte, error) {
	format := nodeFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, n)
	if err != nil {
		return nil, xerrors.Errorf("failed to encode interior node: %v", err)
	}

	return data, nil
}

// LeafNode is a leaf node with a key and a value.
//
// - implements binprefix.TreeNode
type LeafNode struct {
	hash  []byte
	depth uint16
	key   *big.Int
	value []byte
}

// NewLeafNode creates a new leaf node.
func NewLeafNode(depth uint16, key *big.Int, value []byte) *LeafNode {
	return NewLeafNodeWithDigest(depth, key, value, nil)
}

// NewLeafNodeWithDigest creates a new leaf node with its digest.
func NewLeafNodeWithDigest(depth uint16, key *big.Int, value, hash []byte) *LeafNode {
	return &LeafNode{
		hash:  hash,
		depth: depth,
		key:   key,
		value: value,
	}
}

// GetHash implements binprefix.TreeNode. It returns the hash of the node.
func (n *LeafNode) GetHash() []byte {
	return append([]byte{}, n.hash...)
}

// GetDepth returns the depth of the node.
func (n *LeafNode) GetDepth() uint16 {
	return n.depth
}

// GetKey returns the key of the leaf node.
func (n *LeafNode) GetKey() []byte {
	return n.key.Bytes()
}

// GetValue returns the value of the leaf node.
func (n *LeafNode) GetValue() []byte {
	return n.value
}

// GetType implements binprefix.TreeNode. It returns the leaf node type.
func (n *LeafNode) GetType() byte {
	return leafNodeType
}

// Search implements binprefix.TreeNode. It returns the value if the key
// matches.
func (n *LeafNode) Search(key *big.Int, path *Path, b kv.Bucket) ([]byte, error) {
	if path != nil {
		path.value = n.value
	}

	if n.key.Cmp(key) == 0 {
		return n.value, nil
	}

	return nil, nil
}

// Insert implements binprefix.TreeNode. It replaces the leaf node by an
// interior node that contains both the current pair and the new one to insert.
func (n *LeafNode) Insert(key *big.Int, value []byte, b kv.Bucket) (TreeNode, error) {
	if n.key.Cmp(key) == 0 {
		n.hash = nil
		n.value = value
		return n, nil
	}

	prefix := new(big.Int)
	for i := 0; i < int(n.depth); i++ {
		prefix.SetBit(prefix, i, key.Bit(i))
	}

	node := NewInteriorNode(n.depth, prefix)

	// As the node is freshly created, the operations are in-memory and thus it
	// doesn't trigger any error.
	node.Insert(n.key, n.value, b)
	node.Insert(key, value, b)

	return node, nil
}

// Delete implements binprefix.TreeNode. It removes the leaf if the key matches.
func (n *LeafNode) Delete(key *big.Int, b kv.Bucket) (TreeNode, error) {
	if n.key.Cmp(key) == 0 {
		return NewEmptyNode(n.depth, key), nil
	}

	return n, nil
}

// Prepare implements binprefix.TreeNode. It updates the hash of the node if not
// already set and returns the digest.
func (n *LeafNode) Prepare(nonce []byte,
	prefix *big.Int, b kv.Bucket, fac crypto.HashFactory) ([]byte, error) {

	if len(n.hash) > 0 {
		// Hash is already calculated so we can skip and return.
		return n.hash, nil
	}

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

// Visit implements binprefix.TreeNode. It executes the callback with the node.
func (n *LeafNode) Visit(fn func(TreeNode) error) error {
	err := fn(n)
	if err != nil {
		return xerrors.Errorf("visiting leaf: %v", err)
	}

	return nil
}

// Clone implements binprefix.TreeNode. It returns a copy of the leaf node.
func (n *LeafNode) Clone() TreeNode {
	return NewLeafNode(n.depth, n.key, n.value)
}

// Serialize implements serde.Message. It returns the JSON data of the leaf
// node.
func (n *LeafNode) Serialize(ctx serde.Context) ([]byte, error) {
	format := nodeFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, n)
	if err != nil {
		return nil, xerrors.Errorf("failed to encode leaf node: %v", err)
	}

	return data, nil
}

// NodeKey is the key for the node factory.
type NodeKey struct{}

// NodeFactory is the factory to deserialize tree nodes.
//
// - implements serde.Factory
type NodeFactory struct{}

// Deserialize implements serde.Factory. It populates the tree node associated
// to the data if appropriate, otherwise it returns an error.
func (f NodeFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := nodeFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, NodeKey{}, f)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("format failed: %v", err)
	}

	return msg, nil
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
