package pow

import (
	"bytes"

	"go.dedis.ch/dela/core/store/hashtree"
	"golang.org/x/xerrors"
)

// Proof is a proof that the chain of blocks has or has not the key in the
// store. If the key exists, the proof also contains the value.
//
// - implements ordering.Proof
type Proof struct {
	blocks []Block
	path   hashtree.Path
}

// NewProof creates a new valid proof.
func NewProof(blocks []Block, path hashtree.Path) (Proof, error) {
	pr := Proof{
		blocks: blocks,
		path:   path,
	}

	if len(blocks) == 0 {
		return pr, xerrors.New("empty list of blocks")
	}

	last := blocks[len(blocks)-1]
	if !bytes.Equal(last.root, path.GetRoot()) {
		return pr, xerrors.Errorf("mismatch block and share store root %#x != %#x", last.root, path.GetRoot())
	}

	return pr, nil
}

// GetKey implements ordering.Proof.
func (p Proof) GetKey() []byte {
	return p.path.GetKey()
}

// GetValue implements ordering.Proof.
func (p Proof) GetValue() []byte {
	return p.path.GetValue()
}
