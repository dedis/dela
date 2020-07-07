package viewchange

import (
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
)

// ChangeSet is the return of a diff between two authorities.
type ChangeSet interface {
	serde.Message
}

type ChangeSetFactory interface {
	serde.Factory

	ChangeSetOf(serde.Context, []byte) (ChangeSet, error)
}

// ViewChange provides primitives to verify if a participant is allowed to
// propose a block as the leader. Some consensus need a single node to propose
// and the others as backups when it is failing. The index returned announces
// who is allowed to be the leader.
type ViewChange interface {
	GetChangeSetFactory() ChangeSetFactory

	// GetAuthority returns the authority at the given index.
	// TODO: use the proposal ID if we move the blockchain module to be a plugin
	// of the ledger.
	GetAuthority(index uint64) (Authority, error)

	// Wait returns true if the node is the leader for the next proposal.
	Wait() bool

	// Verify returns the authority for the proposal if the address is the
	// correct leader.
	Verify(from mino.Address, index uint64) (Authority, error)
}

// Authority is an extension of the collective authority to provide
// primitives to append new players to it.
type Authority interface {
	serde.Message
	serde.Fingerprinter
	crypto.CollectiveAuthority

	// Apply must apply the change set to the collective authority. It should
	// first remove, then add the new players.
	Apply(ChangeSet) Authority

	// Diff should return the change set to apply to get the given authority.
	Diff(Authority) ChangeSet
}

type AuthorityFactory interface {
	serde.Factory

	AuthorityOf(serde.Context, []byte) (Authority, error)
}
