package types

import (
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
)

// BlockLink is the interface of a link between a previous digest and an actual
// block.
type BlockLink interface {
	serde.Message
	serde.Fingerprinter

	GetHash() Digest

	GetFrom() Digest

	GetTo() Block

	GetPrepareSignature() crypto.Signature

	GetCommitSignature() crypto.Signature

	GetChangeSet() viewchange.ChangeSet
}

// BlockLinkFactory is the interface of the block link factory.
type BlockLinkFactory interface {
	serde.Factory

	BlockLinkOf(serde.Context, []byte) (BlockLink, error)
}
