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
	chain []types.BlockLink
}

func newProof(path hashtree.Path, chain []types.BlockLink) Proof {
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
	if len(p.chain) == 0 {
		return xerrors.New("chain is empty but at least one link is expected")
	}

	authority := genesis.GetRoster()

	prev := genesis.GetHash()

	for _, link := range p.chain {
		// It makes sure that the chain of links is consistent.
		if prev != link.GetFrom() {
			return xerrors.Errorf("mismatch from: '%v' != '%v'", link.GetFrom(), prev)
		}

		// The verifier can be used to verify the signature of the link, but it
		// needs to be created for every link as the roster can change.
		verifier, err := fac.FromAuthority(authority)
		if err != nil {
			return xerrors.Errorf("verifier factory failed: %v", err)
		}

		if link.GetPrepareSignature() == nil || link.GetCommitSignature() == nil {
			return xerrors.New("unexpected nil signature in link")
		}

		// 1. Verify the prepare signature that signs the integrity of the
		// forward link.
		err = verifier.Verify(link.GetHash().Bytes(), link.GetPrepareSignature())
		if err != nil {
			return xerrors.Errorf("invalid prepare signature: %v", err)
		}

		// 2. Verify the commit signature that signs the binary representation
		// of the prepare signature.
		msg, err := link.GetPrepareSignature().MarshalBinary()
		if err != nil {
			return xerrors.Errorf("failed to marshal signature: %v", err)
		}

		err = verifier.Verify(msg, link.GetCommitSignature())
		if err != nil {
			return xerrors.Errorf("invalid commit signature: %v", err)
		}

		prev = link.GetTo().GetHash()

		authority = authority.Apply(link.GetChangeSet())
	}

	last := p.chain[len(p.chain)-1].GetTo()

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
