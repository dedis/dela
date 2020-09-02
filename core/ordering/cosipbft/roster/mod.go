package roster

import (
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/serde"
)

// ChangeSet is the return of a diff between two authorities.
type ChangeSet interface {
	serde.Message

	NumChanges() int

	GetNewAddresses() []mino.Address
}

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

type AuthorityFactory interface {
	serde.Factory

	AuthorityOf(serde.Context, []byte) (Authority, error)
}
