package mem

import (
	"math/big"

	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// DiskNode is an implementation of a tree node which is stored on the
// disk.
//
// - implements mem.TreeNode
type DiskNode struct {
	depth   uint16
	hash    []byte
	context serde.Context
	factory serde.Factory
}

// NewDiskNode creates a new disk node.
func NewDiskNode(depth uint16, ctx serde.Context, factory serde.Factory) *DiskNode {
	return &DiskNode{
		depth:   depth,
		context: ctx,
		factory: factory,
	}
}

// GetHash implements mem.TreeNode. It returns the hash of the disk node if it
// is set, otherwise it returns nil.
func (n *DiskNode) GetHash() []byte {
	return n.hash
}

// GetType returns the type of the node.
func (n *DiskNode) GetType() byte {
	return diskNodeType
}

// Search implements mem.TreeNode. It loads the disk node and then search for
// the key.
func (n *DiskNode) Search(key *big.Int, path *Path, bucket kv.Bucket) ([]byte, error) {
	node, err := n.load(key, bucket)
	if err != nil {
		return nil, err
	}

	value, err := node.Search(key, path, bucket)
	if err != nil {
		return nil, err
	}

	return value, nil
}

// Insert implements mem.TreeNode. It loads the node and inserts the key/value
// pair using in-memory operations. The whole path to the key will be loaded and
// kept in-memory until the tree is persisted.
func (n *DiskNode) Insert(key *big.Int, value []byte, bucket kv.Bucket) (TreeNode, error) {
	node, err := n.load(key, bucket)
	if err != nil {
		return nil, err
	}

	next, err := node.Insert(key, value, bucket)
	if err != nil {
		return nil, err
	}

	return next, nil
}

// Delete implements mem.TreeNode. It loads the node and deletes the key if it
// exists. The whole path to the key is loaded in-memory until the tree is
// persisted.
func (n *DiskNode) Delete(key *big.Int, bucket kv.Bucket) (TreeNode, error) {
	node, err := n.load(key, bucket)
	if err != nil {
		return nil, err
	}

	next, err := node.Delete(key, bucket)
	if err != nil {
		return nil, err
	}

	return next, nil
}

// Prepare implements mem.TreeNode. It loads the node and calculates its hash.
// The subtree might be loaded in-memory if deeper hashes have not been computed
// yet.
func (n *DiskNode) Prepare(nonce []byte, prefix *big.Int,
	bucket kv.Bucket, fac crypto.HashFactory) ([]byte, error) {

	node, err := n.load(prefix, bucket)
	if err != nil {
		return nil, err
	}

	digest, err := node.Prepare(nonce, prefix, bucket, fac)
	if err != nil {
		return nil, err
	}

	err = n.store(prefix, node, bucket)
	if err != nil {
		return nil, err
	}

	n.hash = digest

	return digest, nil
}

// Visit implements mem.TreeNode.
func (n *DiskNode) Visit(fn func(TreeNode) error) error {
	return fn(n)
}

// Clone implements mem.TreeNode. It clones the disk node but both the old and
// the new will read the same bucket.
func (n *DiskNode) Clone() TreeNode {
	return NewDiskNode(n.depth, n.context, n.factory)
}

// Serialize implements serde.Message. It always returns an error as a disk node
// cannot be serialized.
func (n *DiskNode) Serialize(ctx serde.Context) ([]byte, error) {
	return nil, xerrors.New("not implemented")
}

func (n *DiskNode) load(index *big.Int, bucket kv.Bucket) (TreeNode, error) {
	key := n.prepareKey(index)

	data := bucket.Get(key)
	if len(data) == 0 {
		return nil, xerrors.Errorf("prefix %b (%b) not in database", index, key)
	}

	msg, err := n.factory.Deserialize(n.context, data)
	if err != nil {
		return nil, err
	}

	node, ok := msg.(TreeNode)
	if !ok {
		return nil, xerrors.New("invalid tree node")
	}

	return node, nil
}

func (n *DiskNode) store(index *big.Int, node TreeNode, b kv.Bucket) error {
	data, err := node.Serialize(n.context)
	if err != nil {
		return err
	}

	key := n.prepareKey(index)

	err = b.Set(key, data)
	if err != nil {
		return err
	}

	return nil
}

func (n *DiskNode) prepareKey(index *big.Int) []byte {
	prefix := new(big.Int)

	// First fill the prefix until the depth bit which will create a unique key
	// for the node...
	for i := 0; i < int(n.depth); i++ {
		prefix.SetBit(prefix, i, index.Bit(i))
	}

	// ... but we set the bit at _depth_ to one to differentiate prefixes that
	// end with 0s.
	prefix.SetBit(prefix, int(n.depth), 1)

	return prefix.Bytes()
}
