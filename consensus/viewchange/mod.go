package viewchange

import (
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/crypto"
)

// ViewChange provides primitives to verify if a participant is allowed to
// propose a block as the leader.
type ViewChange interface {
	// Wait returns true if the participant is allowed to proceed with the
	// proposal and it returns the rotation if necessary.
	Wait(consensus.Proposal, crypto.CollectiveAuthority) (int, bool)

	// Verify returns the rotation that should be applied for the proposal if
	// any.
	Verify(consensus.Proposal, crypto.CollectiveAuthority) int
}
