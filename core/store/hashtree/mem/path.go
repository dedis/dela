package mem

import (
	"math/big"

	"go.dedis.ch/dela/crypto"
)

// Path is a path along the tree nodes to a key and its value, or none.
//
// - implements hashtree.Path
type Path struct {
	key []byte
	// Root is the root of the hash tree. This value is not serialized and
	// reproduced from the leaf and the interior nodes when deserializing.
	root      []byte
	interiors [][]byte
	leaf      TreeNode
}

// newPath creates an empty path for the provided key. It must be filled to be
// valid.
func newPath(key []byte) Path {
	return Path{
		key: key,
	}
}

// GetKey implements hashtree.Path. It returns the key associated to the path.
func (s Path) GetKey() []byte {
	return s.key
}

// GetValue implements hashtree.Path. It returns the value of the path.
func (s Path) GetValue() []byte {
	switch leaf := s.leaf.(type) {
	case *LeafNode:
		return leaf.value
	default:
		return nil
	}
}

// GetRoot implements hashtree.Path. It returns the root of the path.
func (s Path) GetRoot() []byte {
	return s.root
}

func computeRoot(leaf, key []byte, interiors [][]byte, fac crypto.HashFactory) ([]byte, error) {
	curr := leaf

	bi := new(big.Int)
	bi.SetBytes(key)

	for i := len(interiors) - 1; i >= 0; i-- {
		h := fac.New()

		if bi.Bit(i) == 0 {
			h.Write(curr)
			h.Write(interiors[i])
		} else {
			h.Write(interiors[i])
			h.Write(curr)
		}

		curr = h.Sum(nil)
	}

	return curr, nil
}
