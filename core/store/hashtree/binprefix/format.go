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
			Depth:  node.depth,
			Prefix: node.prefix.Bytes(),
		}

		m = NodeJSON{Empty: &empty}
	case *InteriorNode:
		inter := InteriorNodeJSON{
			Digest: node.GetHash(),
			Depth:  node.depth,
			Prefix: node.prefix.Bytes(),
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

		node := &LeafNode{
			hash:  m.Leaf.Digest,
			depth: m.Leaf.Depth,
			key:   key,
			value: m.Leaf.Value,
		}

		return node, nil
	}

	if m.Empty != nil {
		prefix := new(big.Int)
		prefix.SetBytes(m.Empty.Prefix)

		node := &EmptyNode{
			hash:   m.Empty.Digest,
			depth:  m.Empty.Depth,
			prefix: prefix,
		}

		return node, nil
	}

	if m.Interior != nil {
		factory := ctx.GetFactory(NodeKey{})

		prefix := new(big.Int)
		prefix.SetBytes(m.Interior.Prefix)

		node := &InteriorNode{
			hash:   m.Interior.Digest,
			depth:  m.Interior.Depth,
			prefix: prefix,
			left:   NewDiskNode(m.Interior.Depth+1, nil, ctx, factory),
			right:  NewDiskNode(m.Interior.Depth+1, nil, ctx, factory),
		}

		return node, nil
	}

	return nil, xerrors.New("message is empty")
}
