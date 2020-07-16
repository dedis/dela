package mem

// Path is a path along the tree nodes to a key and its value, or none.
type Path struct {
	key       []byte
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

// GetKey returns the key associated to the path.
func (s Path) GetKey() []byte {
	return s.key
}

// GetValue returns the value of the path.
func (s Path) GetValue() []byte {
	switch leaf := s.leaf.(type) {
	case *LeafNode:
		return leaf.value
	default:
		return nil
	}
}

// GetRoot returns the root of the path.
func (s Path) GetRoot() []byte {
	if len(s.interiors) == 0 {
		switch leaf := s.leaf.(type) {
		case *EmptyNode:
			return leaf.hash
		case *LeafNode:
			return leaf.hash
		default:
			return nil
		}
	}

	return s.interiors[0]
}
