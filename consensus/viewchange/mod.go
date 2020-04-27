package viewchange

import (
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/mino"
)

// ViewChange provides primitives to verify if a participant is allowed to
// propose a block as the leader.
type ViewChange interface {
	// Wait returns true if the participant is allowed to proceed with the
	// proposal and it returns the rotation if necessary.
	Wait(consensus.Proposal, crypto.CollectiveAuthority) (uint32, bool)

	// Verify returns the rotation that should be applied for the proposal if
	// any.
	Verify(consensus.Proposal, crypto.CollectiveAuthority) uint32
}

// Player is a tuple of an address and its public key.
type Player struct {
	Address   mino.Address
	PublicKey crypto.PublicKey
}

// ChangeSet is a combination of changes of a collective authority.
type ChangeSet struct {
	Remove []Player
	Add    []Player
	Leader uint32
}

// EvolvableAuthority is an extension of the collective authority to provide
// primitives to append new players to it.
type EvolvableAuthority interface {
	crypto.CollectiveAuthority

	Apply(ChangeSet) EvolvableAuthority
}

// Governance is an interface to get information about the collective authority
// of a proposal.
type Governance interface {
	// GetAuthority must return the authority that governs the block. It will be
	// used to sign the forward link to the next proposal.
	GetAuthority(index uint64) (EvolvableAuthority, error)

	// GetChangeSet must return the changes to the authority that will be
	// applied for the next proposal.
	GetChangeSet(index uint64) ChangeSet
}
