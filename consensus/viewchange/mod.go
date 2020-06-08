package viewchange

import (
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/encoding"
	"go.dedis.ch/dela/mino"
)

// ViewChange provides primitives to verify if a participant is allowed to
// propose a block as the leader. Some consensus need a single node to propose
// and the others as backups when it is failing. The index returned announces
// who is allowed to be the leader.
type ViewChange interface {
	// GetAuthority returns the authority at the given index.
	// TODO: use the proposal ID if we move the blockchain module to be a plugin
	// of the ledger.
	GetAuthority(index uint64) (Authority, error)

	// Wait returns true if the participant is allowed to proceed with the
	// proposal. It also returns the participant index if true.
	Wait() bool

	// Verify returns the leader index for that proposal.
	Verify(from mino.Address, index uint64) (Authority, error)
}

// Player is a tuple of an address and its public key.
type Player struct {
	Address   mino.Address
	PublicKey crypto.PublicKey
}

// ChangeSet is a combination of changes of a collective authority.
type ChangeSet struct {
	Remove []uint32
	Add    []Player
}

// Authority is an extension of the collective authority to provide
// primitives to append new players to it.
type Authority interface {
	encoding.Packable
	crypto.CollectiveAuthority

	// Apply must apply the change set to the collective authority. It should
	// first remove, then add the new players.
	Apply(ChangeSet) Authority

	// Diff should return the change set to apply to get the given authority.
	Diff(Authority) ChangeSet
}
