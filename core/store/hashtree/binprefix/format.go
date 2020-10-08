package binprefix

import (
	"math/big"

	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// LeafNodeJSON is the JSON representation of a leaf node.
type LeafNodeJSON struct {
	Digest []byte
	Depth  uint16
	Prefix []byte
	Value  []byte
}

// InteriorNodeJSON is the JSON representation of an interior node.
type InteriorNodeJSON struct {
	Digest []byte
	Depth  uint16
	Prefix []byte
}

// EmptyNodeJSON is the JSON representation of an empty node.
type EmptyNodeJSON struct {
	Digest []byte
	Depth  uint16
	Prefix []byte
}

// NodeJSON is the wrapper around the different types of a tree node.
type NodeJSON struct {
	Leaf     *LeafNodeJSON     `json:",omitempty"`
	Interior *InteriorNodeJSON `json:",omitempty"`
	Empty    *EmptyNodeJSON    `json:",omitempty"`
}

type nodeFormat struct{}

func (f nodeFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	var m NodeJSON

	switch node := msg.(type) {
	case *LeafNode:
		leaf := LeafNodeJSON{
			Digest: node.GetHash(),
			Depth:  node.GetDepth(),
			Prefix: node.GetKey(),
			Value:  node.GetValue(),
		}

		m = NodeJSON{Leaf: &leaf}
	case *EmptyNode:
		empty := EmptyNodeJSON{
			Digest: node.GetHash(),
			Depth:  node.GetDepth(),
			Prefix: node.GetPrefix().Bytes(),
		}

		m = NodeJSON{Empty: &empty}
	case *InteriorNode:
		inter := InteriorNodeJSON{
			Digest: node.GetHash(),
			Depth:  node.GetDepth(),
			Prefix: node.GetPrefix().Bytes(),
		}

		m = NodeJSON{Interior: &inter}
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("failed to marshal: %v", err)
	}

	return data, nil
}

func (f nodeFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := NodeJSON{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("failed to unmarshal: %v", err)
	}

	if m.Leaf != nil {
		key := new(big.Int)
		key.SetBytes(m.Leaf.Prefix)

		node := NewLeafNodeWithDigest(m.Leaf.Depth, key, m.Leaf.Value, m.Leaf.Digest)

		return node, nil
	}

	if m.Empty != nil {
		prefix := new(big.Int)
		prefix.SetBytes(m.Empty.Prefix)

		node := NewEmptyNodeWithDigest(m.Empty.Depth, prefix, m.Empty.Digest)

		return node, nil
	}

	if m.Interior != nil {
		factory := ctx.GetFactory(NodeKey{})

		prefix := new(big.Int)
		prefix.SetBytes(m.Interior.Prefix)

		node := NewInteriorNodeWithChildren(
			m.Interior.Depth,
			prefix,
			m.Interior.Digest,
			NewDiskNode(m.Interior.Depth+1, nil, ctx, factory),
			NewDiskNode(m.Interior.Depth+1, nil, ctx, factory),
		)

		return node, nil
	}

	return nil, xerrors.New("message is empty")
}
