package authority

import (
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
)

// ChangeSet is the return of a diff between two authorities.
type ChangeSet interface {
	serde.Message

	// NumChanges returns the number of changes that will be applied with this
	// change set.
	NumChanges() int

	// GetNewAddresses returns the list of addresses for the new members.
	GetNewAddresses() []mino.Address
}

// ChangeSetFactory is the factory interface to deserialize change sets.
type ChangeSetFactory interface {
	serde.Factory

	ChangeSetOf(serde.Context, []byte) (ChangeSet, error)
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

// Factory is the factory interface to deserialize authorities.
type Factory interface {
	serde.Factory

	AuthorityOf(serde.Context, []byte) (Authority, error)
}
