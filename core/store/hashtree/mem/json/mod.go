package json

type LeafNodeJSON struct {
	Digest []byte
	Depth  uint16
	Key    []byte
	Value  []byte
}
