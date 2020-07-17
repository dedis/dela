package mem

import (
	"math/big"

	"go.dedis.ch/dela/crypto"
)

// Path is a path from the root to a leaf, represented as a series of interior
// nodes hashes. The end of the path is either a leaf with a key holding a
// value, or an empty node.
//
// - implements hashtree.Path
type Path struct {
	nonce []byte
	key   []byte
	value []byte
	// Root is the root of the hash tree. This value is not serialized and
	// reproduced from the leaf and the interior nodes when deserializing.
	root      []byte
	interiors [][]byte
}

// newPath creates an empty path for the provided key. It must be filled to be
// valid.
func newPath(nonce, key []byte) Path {
	return Path{
		nonce: nonce,
		key:   key,
	}
}

// GetKey implements hashtree.Path. It returns the key associated to the path.
func (s Path) GetKey() []byte {
	return s.key
}

// GetValue implements hashtree.Path. It returns the value pointed by the path.
func (s Path) GetValue() []byte {
	return s.value
}

// GetRoot implements hashtree.Path. It returns the hash of the root node
// calculated from the leaf up to the root.
func (s Path) GetRoot() []byte {
	return s.root
}

func (s Path) computeRoot(fac crypto.HashFactory) ([]byte, error) {
	bi := new(big.Int)
	bi.SetBytes(s.key)

	var node TreeNode
	if s.value != nil {
		node = NewLeafNode(uint16(len(s.interiors)), s.key, s.value)
	} else {
		node = NewEmptyNode(uint16(len(s.interiors)))
	}

	// Reproduce the shortest unique prefix for the key.
	prefix := new(big.Int)
	for i := 0; i < len(s.interiors); i++ {
		prefix.SetBit(prefix, i, bi.Bit(i))
	}

	curr, err := node.Prepare(s.nonce, prefix, fac)
	if err != nil {
		return nil, err
	}

	for i := len(s.interiors) - 1; i >= 0; i-- {
		h := fac.New()

		if bi.Bit(i) == 0 {
			h.Write(curr)
			h.Write(s.interiors[i])
		} else {
			h.Write(s.interiors[i])
			h.Write(curr)
		}

		curr = h.Sum(nil)
	}

	return curr, nil
}
