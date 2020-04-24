package viewchange

import (
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/crypto"
)

// ViewChange provides primitives to verify if a participant is allowed to
// propose a block as the leader. It is also responsible for verifying the
// integrity of the players of the chain.
type ViewChange interface {
	// Wait returns a nil error when the player is allowed to propose the block.
	// It will also return the authorized list of players that must be used so
	// that the Verify function returns nil.
	//
	// Note: the implementation of the returned mino.Players interface must be
	// preserved.
	Wait(consensus.Proposal, crypto.CollectiveAuthority) (int, bool)

	// Verify makes sure that the players for the given are authorized and in
	// the right order if necessary.
	Verify(consensus.Proposal, crypto.CollectiveAuthority) int
}
