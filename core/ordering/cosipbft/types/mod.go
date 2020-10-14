// Package types implements the network messages for cosipbft.
//
// The messages are implemented in a different package to prevent cycle imports
// when importing the serde formats.
//
// Documentation Last Review: 13.10.2020
//
package types

import (
	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
)

// Link is the interface of a link between two blocks.
type Link interface {
	serde.Message
	serde.Fingerprinter

	// GetHash returns the digest of the forward link that is signed with the
	// prepare signature.
	GetHash() Digest

	// GetFrom returns the digest of the previous block.
	GetFrom() Digest

	// GetTo returns the digest of the block the link is pointing at.
	GetTo() Digest

	// GetPrepareSignature returns the signature that proves the integrity of
	// the link.
	GetPrepareSignature() crypto.Signature

	// GetCommitSignature returns the signature that proves the block has been
	// committed.
	GetCommitSignature() crypto.Signature

	// GetChangeSet returns the roster change set for this link.
	GetChangeSet() authority.ChangeSet
}

// BlockLink is an extension of the Link interface to include the block the link
// is pointing at. It also provides a function to get a lighter link without the
// block.
type BlockLink interface {
	Link

	// GetBlock returns the block the link is pointing at.
	GetBlock() Block

	// Reduce returns the forward link equivalent to this block link but without
	// the block to allow a lighter serialization.
	Reduce() Link
}

// LinkFactory is the interface of the block link factory.
type LinkFactory interface {
	serde.Factory

	LinkOf(serde.Context, []byte) (Link, error)

	BlockLinkOf(serde.Context, []byte) (BlockLink, error)
}

// Chain is the interface to combine several links in order to create a chain
// that can prove the integrity of the blocks from the genesis block.
type Chain interface {
	serde.Message

	// GetLinks returns all the links that defines the chain in order.
	GetLinks() []Link

	// GetBlock returns the latest block that the chain is pointing at.
	GetBlock() Block

	// Verify takes the genesis block and the verifier factory that should
	// verify the chain.
	Verify(genesis Genesis, fac crypto.VerifierFactory) error
}

// ChainFactory is the interface to serialize and deserialize chains.
type ChainFactory interface {
	serde.Factory

	// ChainOf returns the chain from the data if appropriate, otherwise it
	// returns an error.
	ChainOf(serde.Context, []byte) (Chain, error)
}
