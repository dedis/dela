package pow

import (
	"bytes"

	"go.dedis.ch/dela/core/store/trie"
	"golang.org/x/xerrors"
)

// Proof is a proof that the chain of blocks has or has not the key in the
// store. If the key exists, the proof also contains the value.
//
// - implements ordering.Proof
type Proof struct {
	blocks []Block
	share  trie.Share
}

// NewProof creates a new valid proof.
func NewProof(blocks []Block, share trie.Share) (Proof, error) {
	pr := Proof{
		blocks: blocks,
		share:  share,
	}

	if len(blocks) == 0 {
		return pr, xerrors.New("empty list of blocks")
	}

	last := blocks[len(blocks)-1]
	if !bytes.Equal(last.root, share.GetRoot()) {
		return pr, xerrors.Errorf("mismatch block and share store root %#x != %#x", last.root, share.GetRoot())
	}

	return pr, nil
}

// GetKey implements ordering.Proof.
func (p Proof) GetKey() []byte {
	return p.share.GetKey()
}

// GetValue implements ordering.Proof.
func (p Proof) GetValue() []byte {
	return p.share.GetValue()
}
