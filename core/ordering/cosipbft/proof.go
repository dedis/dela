// This file contains the implementation of a proof for this ordering service.
//
// Documentation Last Review: 12.10.2020
//

package cosipbft

import (
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/core/store/hashtree"
	"go.dedis.ch/dela/crypto"
	"golang.org/x/xerrors"
)

// Proof is a combination of elements that will prove the inclusion or the
// absence of a key/value pair in the given block.
//
// - implements ordering.Proof
type Proof struct {
	path  hashtree.Path
	chain types.Chain
}

func newProof(path hashtree.Path, chain types.Chain) Proof {
	return Proof{
		path:  path,
		chain: chain,
	}
}

// GetKey implements ordering.Proof. It returns the key associated to the proof.
func (p Proof) GetKey() []byte {
	return p.path.GetKey()
}

// GetValue implements ordering.Proof. It returns the value associated to the
// proof if the key exists, otherwise it returns nil.
func (p Proof) GetValue() []byte {
	return p.path.GetValue()
}

// Verify takes the genesis block and the verifier factory to verify the chain
// up to the latest block.
func (p Proof) Verify(genesis types.Genesis, fac crypto.VerifierFactory) error {
	err := p.chain.Verify(genesis, fac)
	if err != nil {
		return xerrors.Errorf("failed to verify chain: %v", err)
	}

	last := p.chain.GetBlock()

	// The path object is transmitted with enough information so that when it is
	// instanciated, it can calculate the Merkle root. It is therefore
	// unnecessary to do it again here.
	root := types.Digest{}
	copy(root[:], p.path.GetRoot())

	// The Merkle root must match the one stored in the block to prove that the
	// chain is correct.
	if last.GetTreeRoot() != root {
		return xerrors.Errorf("mismatch tree root: '%v' != '%v'",
			last.GetTreeRoot(), root)
	}

	return nil
}
