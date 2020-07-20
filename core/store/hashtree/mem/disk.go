package mem

import (
	"math/big"

	"go.dedis.ch/dela/core/store/kv"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
	"golang.org/x/xerrors"
)

// DiskNode is an implementation of a tree node which is stored on the
// disk. It is the barrier between in-memory operations and persistent ones.
type DiskNode struct {
	depth uint16
	hash  []byte
}

func NewDiskNode(depth uint16) *DiskNode {
	return &DiskNode{
		depth: depth,
	}
}

func (n *DiskNode) GetHash() []byte {
	return n.hash
}

func (n *DiskNode) GetType() byte {
	return diskNodeType
}

// Search implements mem.TreeNode.
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

func (n *DiskNode) Visit(fn func(TreeNode)) {
	fn(n)
}

func (n *DiskNode) Clone() TreeNode {
	return NewDiskNode(n.depth)
}

func (n *DiskNode) Serialize(ctx serde.Context) ([]byte, error) {
	return nil, xerrors.New("not implemented")
}

func (n *DiskNode) load(index *big.Int, bucket kv.Bucket) (TreeNode, error) {
	key := n.prepareKey(index)

	data := bucket.Get(key)
	if len(data) == 0 {
		return nil, xerrors.Errorf("prefix %b not in database", key)
	}

	ctx := json.NewContext()
	factory := NodeFactory{}

	msg, err := factory.Deserialize(ctx, data)
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
	ctx := json.NewContext()

	data, err := node.Serialize(ctx)
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
	for i := 0; i <= int(n.depth); i++ {
		prefix.SetBit(prefix, i, index.Bit(i))
	}

	// ... but we set the bit at depth+1 to one to differentiate prefixes that
	// end with 0s.
	prefix.SetBit(prefix, int(n.depth)+1, 1)

	return prefix.Bytes()
}
