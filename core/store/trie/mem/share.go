package mem

// TODO: implement merkle trie root proof.
type share struct {
	key   []byte
	value []byte
	root  []byte
}

// TODO: constructor should calculate the root so that the deserialization does
// not use the root.
func newShare(key, value, root []byte) share {
	return share{
		key:   key,
		value: value,
		root:  root,
	}
}

func (s share) GetKey() []byte {
	return s.key
}

func (s share) GetValue() []byte {
	return s.value
}

func (s share) GetRoot() []byte {
	return s.root
}
