// This file contains the implementation of a chain of block links.
//
// Documentation Last Review: 13.10.2020
//

package types

import (
	"io"

	"go.dedis.ch/dela/core/ordering/cosipbft/authority"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
)

var (
	chainFormats = registry.NewSimpleRegistry()
	linkFormats  = registry.NewSimpleRegistry()
)

// RegisterLinkFormat registers the engine for the provided format.
func RegisterLinkFormat(f serde.Format, e serde.FormatEngine) {
	linkFormats.Register(f, e)
}

// RegisterChainFormat registers the engine for the provided format.
func RegisterChainFormat(f serde.Format, e serde.FormatEngine) {
	chainFormats.Register(f, e)
}

// ForwardLink is a link between two blocks that is only using their different
// digests to reduce the serialization footprint.
//
// - implements types.Link
// - implements serde.Fingerprinter
type forwardLink struct {
	digest     Digest
	from       Digest
	to         Digest
	changeset  authority.ChangeSet
	prepareSig crypto.Signature
	commitSig  crypto.Signature
}

type linkTemplate struct {
	forwardLink

	hashFac crypto.HashFactory
}

// LinkOption is the type of option to set some optional fields of the
// link.
type LinkOption func(*linkTemplate)

// WithSignatures is the option to set the signatures of the link.
func WithSignatures(prep, commit crypto.Signature) LinkOption {
	return func(tmpl *linkTemplate) {
		tmpl.prepareSig = prep
		tmpl.commitSig = commit
	}
}

// WithChangeSet is the option to set the change set of the roster for this
// link.
func WithChangeSet(cs authority.ChangeSet) LinkOption {
	return func(tmpl *linkTemplate) {
		tmpl.changeset = cs
	}
}

// WithLinkHashFactory is the option to set the hash factory for the link.
func WithLinkHashFactory(fac crypto.HashFactory) LinkOption {
	return func(tmpl *linkTemplate) {
		tmpl.hashFac = fac
	}
}

// NewForwardLink creates a new forward link between the two block digests.
func NewForwardLink(from, to Digest, opts ...LinkOption) (Link, error) {
	tmpl := linkTemplate{
		forwardLink: forwardLink{
			from:      from,
			to:        to,
			changeset: authority.NewChangeSet(),
		},
		hashFac: crypto.NewSha256Factory(),
	}

	for _, opt := range opts {
		opt(&tmpl)
	}

	h := tmpl.hashFac.New()
	err := tmpl.Fingerprint(h)
	if err != nil {
		return nil, xerrors.Errorf("failed to fingerprint: %v", err)
	}

	copy(tmpl.digest[:], h.Sum(nil))

	return tmpl.forwardLink, nil
}

// GetHash implements types.Link. It returns the digest of the link.
func (link forwardLink) GetHash() Digest {
	return link.digest
}

// GetFrom implements types.Link. It returns the digest of the source block.
func (link forwardLink) GetFrom() Digest {
	return link.from
}

// GetTo implements types.Link. It returns the block the link is pointing to.
func (link forwardLink) GetTo() Digest {
	return link.to
}

// GetPrepareSignature implements types.Link. It returns the prepare signature
// if it is set, otherwise it returns nil.
func (link forwardLink) GetPrepareSignature() crypto.Signature {
	return link.prepareSig
}

// GetCommitSignature implements types.Link. It returns the commit signature if
// it is set, otherwise it returns nil.
func (link forwardLink) GetCommitSignature() crypto.Signature {
	return link.commitSig
}

// GetChangeSet implements types.Link. It returns the change set of the roster
// for this link.
func (link forwardLink) GetChangeSet() authority.ChangeSet {
	return link.changeset
}

// Fingerprint implements serde.Fingerprinter. It deterministically writes a
// binary representation of the block link.
func (link forwardLink) Fingerprint(w io.Writer) error {
	_, err := w.Write(link.from[:])
	if err != nil {
		return xerrors.Errorf("couldn't write from: %v", err)
	}

	id := link.GetTo()

	_, err = w.Write(id[:])
	if err != nil {
		return xerrors.Errorf("couldn't write to: %v", err)
	}

	return nil
}

// Serialize implements serde.Message. It returns the data of the serialized
// forward link.
func (link forwardLink) Serialize(ctx serde.Context) ([]byte, error) {
	format := linkFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, link)
	if err != nil {
		return nil, xerrors.Errorf("encoding link failed: %v", err)
	}

	return data, nil
}

// BlockLink is a link between two blocks but only keep the previous block
// digest.
//
// - implements types.BlockLink
type blockLink struct {
	forwardLink

	block Block
}

// NewBlockLink creates a new block link between from and to.
func NewBlockLink(from Digest, to Block, opts ...LinkOption) (BlockLink, error) {
	link, err := NewForwardLink(from, to.digest, opts...)
	if err != nil {
		return nil, xerrors.Errorf("creating forward link: %v", err)
	}

	bl := blockLink{
		forwardLink: link.(forwardLink),
		block:       to,
	}

	return bl, nil
}

// GetBlock implements types.BlockLink. It returns the block that the link is
// pointing at.
func (link blockLink) GetBlock() Block {
	return link.block
}

// Reduce implements types.BlockLink. It reduces the block link to its
// minimalistic shape.
func (link blockLink) Reduce() Link {
	return link.forwardLink
}

// Serialize implements serde.Message. It returns the serialized data for this
// block link.
func (link blockLink) Serialize(ctx serde.Context) ([]byte, error) {
	format := linkFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, link)
	if err != nil {
		return nil, xerrors.Errorf("encoding failed: %v", err)
	}

	return data, nil
}

// ChangeSetKey is the key of the change set factory.
type ChangeSetKey struct{}

// BlockLinkFac is the factory to deserialize block link messages.
//
// - implements types.LinkFactory
type linkFac struct {
	blockFac serde.Factory
	sigFac   crypto.SignatureFactory
	csFac    authority.ChangeSetFactory
}

// NewLinkFactory creates a new block link factory.
func NewLinkFactory(blockFac serde.Factory,
	sigFac crypto.SignatureFactory, csFac authority.ChangeSetFactory) LinkFactory {

	return linkFac{
		blockFac: blockFac,
		sigFac:   sigFac,
		csFac:    csFac,
	}
}

// Deserialize implements serde.Factory. It populates the block link if
// appropriate, otherwise it returns an error.
func (fac linkFac) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := linkFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, BlockKey{}, fac.blockFac)
	ctx = serde.WithFactory(ctx, AggregateKey{}, fac.sigFac)
	ctx = serde.WithFactory(ctx, ChangeSetKey{}, fac.csFac)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("decoding link failed: %v", err)
	}

	return msg, nil
}

// LinkOf implements types.LinkFactory. It populates the link if appropriate,
// otherwise it returns an error.
func (fac linkFac) LinkOf(ctx serde.Context, data []byte) (Link, error) {
	msg, err := fac.Deserialize(ctx, data)
	if err != nil {
		return nil, err
	}

	link, ok := msg.(Link)
	if !ok {
		return nil, xerrors.Errorf("invalid forward link '%T'", msg)
	}

	return link, nil
}

// BlockLinkOf implements types.LinkFactory. It populates the block link if
// appropriate, otherwise it returns an error.
func (fac linkFac) BlockLinkOf(ctx serde.Context, data []byte) (BlockLink, error) {
	msg, err := fac.Deserialize(ctx, data)
	if err != nil {
		return nil, err
	}

	link, ok := msg.(BlockLink)
	if !ok {
		return nil, xerrors.Errorf("invalid block link '%T'", msg)
	}

	return link, nil
}

// Chain is a combination of ordered links that will define a proof of existence
// for a block. It does not include the genesis block which is assumed to be
// known beforehands.
//
// - implements types.Chain
type chain struct {
	last  BlockLink
	prevs []Link
}

// NewChain creates a new chain from the block link and the previous forward
// links.
func NewChain(last BlockLink, prevs []Link) Chain {
	return chain{
		last:  last,
		prevs: prevs,
	}
}

// GetLinks implements types.Chain. It returns all the links of the chain in
// order.
func (c chain) GetLinks() []Link {
	return append(append([]Link{}, c.prevs...), c.last)
}

// GetBlock implements types.Chain. It returns the block the chain is pointing
// at.
func (c chain) GetBlock() Block {
	return c.last.GetBlock()
}

// Verify implements types.Chain. It verifies the integrity of the chain using
// the genesis block and the verifier factory.
func (c chain) Verify(genesis Genesis, fac crypto.VerifierFactory) error {
	authority := genesis.GetRoster()

	prev := genesis.GetHash()

	for _, link := range c.GetLinks() {
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

		if link.GetPrepareSignature() == nil {
			return xerrors.New("unexpected nil prepare signature in link")
		}

		if link.GetCommitSignature() == nil {
			return xerrors.New("unexpected nil commit signature in link")
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

		prev = link.GetTo()

		authority = authority.Apply(link.GetChangeSet())
	}

	return nil
}

// Serialize implements serde.Message. It returns the data of the serialized
// chain.
func (c chain) Serialize(ctx serde.Context) ([]byte, error) {
	format := chainFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, c)
	if err != nil {
		return nil, xerrors.Errorf("encoding chain failed: %v", err)
	}

	return data, nil
}

// ChainFactory is a factory to serialize and deserialize a chain.
//
// - implements types.ChainFactory
type chainFactory struct {
	linkFac LinkFactory
}

// NewChainFactory creates a new factory from the link factory.
func NewChainFactory(fac LinkFactory) ChainFactory {
	return chainFactory{
		linkFac: fac,
	}
}

// Deserialize implements serde.Factory. It returns the chain from the data if
// appropriate, otherwise it returns an error.
func (fac chainFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	return fac.ChainOf(ctx, data)
}

// ChainOf implements types.ChainFactory. It returns the chain from the data if
// appropriate, otherwise it returns an error.
func (fac chainFactory) ChainOf(ctx serde.Context, data []byte) (Chain, error) {
	format := chainFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, LinkKey{}, fac.linkFac)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("decoding chain failed: %v", err)
	}

	chain, ok := msg.(Chain)
	if !ok {
		return nil, xerrors.Errorf("invalid chain '%T'", msg)
	}

	return chain, nil
}
