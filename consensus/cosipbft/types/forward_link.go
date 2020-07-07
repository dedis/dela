package types

import (
	"bytes"
	"io"

	"go.dedis.ch/dela/consensus"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
)

var (
	linkFormats  = registry.NewSimpleRegistry()
	chainFormats = registry.NewSimpleRegistry()
)

// RegisterForwardLinkFormat registers the engine for the provided format.
func RegisterForwardLinkFormat(c serde.Format, f serde.FormatEngine) {
	linkFormats.Register(c, f)
}

// RegisterChainFormat registers the engine for the provided format.
func RegisterChainFormat(c serde.Format, f serde.FormatEngine) {
	chainFormats.Register(c, f)
}

// Digest is an alias for the bytes type for hash.
type Digest = []byte

// ForwardLink is the cryptographic primitive to ensure a block is a successor
// of a previous one.
//
// - implements serde.Message
type ForwardLink struct {
	hash Digest
	from Digest
	to   Digest
	// prepare is the signature of the combination of From and To to prove that
	// the nodes agreed on a valid forward link between the two blocks.
	prepare crypto.Signature
	// commit is the signature of the Prepare signature to prove that a
	// threshold of the nodes have committed the block.
	commit crypto.Signature
	// changeset announces the changes in the authority for the next proposal.
	changeset viewchange.ChangeSet
}

type forwardLinkTemplate struct {
	ForwardLink
	factory crypto.HashFactory
}

// ForwardLinkOption is an option to set a forward link value when creating one.
type ForwardLinkOption func(*forwardLinkTemplate)

// WithPrepare is an option to set the prepare phase signature.
func WithPrepare(s crypto.Signature) ForwardLinkOption {
	return func(link *forwardLinkTemplate) {
		link.prepare = s
	}
}

// WithCommit is an option to set the commit phase signature.
func WithCommit(s crypto.Signature) ForwardLinkOption {
	return func(link *forwardLinkTemplate) {
		link.commit = s
	}
}

// WithChangeSet is an option to set the change set.
func WithChangeSet(cs viewchange.ChangeSet) ForwardLinkOption {
	return func(link *forwardLinkTemplate) {
		link.changeset = cs
	}
}

// WithHashFactory is an option to use a different hash factory that the default
// SHA-256 one.
func WithHashFactory(f crypto.HashFactory) ForwardLinkOption {
	return func(link *forwardLinkTemplate) {
		link.factory = f
	}
}

// NewForwardLink creates a new forward link between from and to using the
// options to set particular values.
func NewForwardLink(from, to Digest, opts ...ForwardLinkOption) (ForwardLink, error) {
	tmpl := &forwardLinkTemplate{
		ForwardLink: ForwardLink{
			from: from,
			to:   to,
		},
		factory: crypto.NewSha256Factory(),
	}

	for _, opt := range opts {
		opt(tmpl)
	}

	h := tmpl.factory.New()
	err := tmpl.Fingerprint(h)
	if err != nil {
		return ForwardLink{}, xerrors.Errorf("couldn't fingerprint: %v", err)
	}

	tmpl.hash = h.Sum(nil)

	return tmpl.ForwardLink, nil
}

// GetFrom returns the identifier the forward link is pointing at.
func (fl ForwardLink) GetFrom() []byte {
	return append([]byte{}, fl.from...)
}

// GetTo returns the identifier the forward link is pointing at.
func (fl ForwardLink) GetTo() []byte {
	return append([]byte{}, fl.to...)
}

// GetFingerprint returns the fingerprint of the forward link.
func (fl ForwardLink) GetFingerprint() []byte {
	return fl.hash
}

// GetPrepareSignature returns the prepare phase signature.
func (fl ForwardLink) GetPrepareSignature() crypto.Signature {
	return fl.prepare
}

// GetCommitSignature returns the commit phase signature.
func (fl ForwardLink) GetCommitSignature() crypto.Signature {
	return fl.commit
}

// GetChangeSet returns the change set.
func (fl ForwardLink) GetChangeSet() viewchange.ChangeSet {
	return fl.changeset
}

// Verify makes sure the signatures of the forward link are correct.
func (fl ForwardLink) Verify(v crypto.Verifier) error {
	err := v.Verify(fl.hash, fl.prepare)
	if err != nil {
		return xerrors.Errorf("couldn't verify prepare signature: %w", err)
	}

	buffer, err := fl.prepare.MarshalBinary()
	if err != nil {
		return xerrors.Errorf("couldn't marshal the signature: %w", err)
	}

	err = v.Verify(buffer, fl.commit)
	if err != nil {
		return xerrors.Errorf("couldn't verify commit signature: %w", err)
	}

	return nil
}

// Serialize implements serde.Message.
func (fl ForwardLink) Serialize(ctx serde.Context) ([]byte, error) {
	format := linkFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, fl)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode link: %v", err)
	}

	return data, nil
}

// Fingerprint deterministacally writes a binary representation of the forward
// link into the writer.
func (fl ForwardLink) Fingerprint(w io.Writer) error {
	_, err := w.Write(fl.from)
	if err != nil {
		return xerrors.Errorf("couldn't write 'from': %v", err)
	}
	_, err = w.Write(fl.to)
	if err != nil {
		return xerrors.Errorf("couldn't write 'to': %v", err)
	}

	return nil
}

// Chain is a chain of forward links that can prove the correctness of a
// proposal.
//
// - implements consensus.Chain - implements serde.Message
type Chain struct {
	links []ForwardLink
}

// NewChain creates a new chain of forward links
func NewChain(links ...ForwardLink) Chain {
	return Chain{links: links}
}

// Len returns the length of the chain.
func (c Chain) Len() int {
	return len(c.links)
}

// GetLinks returns the ordered list of forward links.
func (c Chain) GetLinks() []ForwardLink {
	return append([]ForwardLink{}, c.links...)
}

// GetTo implements consensus.Chain. It returns the last hash of the chain.
func (c Chain) GetTo() []byte {
	if len(c.links) == 0 {
		return nil
	}

	return c.links[len(c.links)-1].to
}

// Serialize implements serde.Message.
func (c Chain) Serialize(ctx serde.Context) ([]byte, error) {
	format := chainFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, c)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode chain: %v", err)
	}

	return data, nil
}

// ChangeSetKeyFac is the key of the change set factory.
type ChangeSetKeyFac struct{}

// ChainFactory is an implementation of the chainFactory interface for forward
// links.
//
// - implements consensus.ChainFactory
// - implements serde.Factory
type ChainFactory struct {
	sigFactory      crypto.SignatureFactory
	csFactory       viewchange.ChangeSetFactory
	viewchange      viewchange.ViewChange
	verifierFactory crypto.VerifierFactory
}

// NewChainFactory creates a new chain factory.
func NewChainFactory(
	sf crypto.SignatureFactory,
	csf viewchange.ChangeSetFactory,
	vc viewchange.ViewChange,
	vf crypto.VerifierFactory) ChainFactory {

	return ChainFactory{
		sigFactory:      sf,
		csFactory:       csf,
		viewchange:      vc,
		verifierFactory: vf,
	}
}

// Deserialize implements serde.Factory.
func (f ChainFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	return f.ChainOf(ctx, data)
}

// ChainOf implements consensus.ChainFactory. It looks up the format and
// populates the chain if appropriate, otherwise it returns an error.
func (f ChainFactory) ChainOf(ctx serde.Context, data []byte) (consensus.Chain, error) {
	format := chainFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, CoSigKeyFac{}, f.sigFactory)
	ctx = serde.WithFactory(ctx, ChangeSetKeyFac{}, f.csFactory)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode chain: %v", err)
	}

	chain, ok := msg.(Chain)
	if !ok {
		return nil, xerrors.Errorf("invalid message of type '%T'", msg)
	}

	err = f.verify(chain)
	if err != nil {
		return nil, xerrors.Errorf("couldn't verify the chain: %v", err)
	}

	return chain, nil
}

func (f ChainFactory) verify(chain Chain) error {
	if len(chain.links) == 0 {
		return nil
	}

	authority, err := f.viewchange.GetAuthority(0)
	if err != nil {
		return xerrors.Errorf("couldn't get authority: %v", err)
	}

	lastIndex := len(chain.links) - 1
	for i, link := range chain.links {
		verifier, err := f.verifierFactory.FromAuthority(authority)
		if err != nil {
			return xerrors.Errorf("couldn't create the verifier: %v", err)
		}

		err = link.Verify(verifier)
		if err != nil {
			return xerrors.Errorf("couldn't verify link %d: %w", i, err)
		}

		if i != lastIndex && !bytes.Equal(link.to, chain.links[i+1].from) {
			return xerrors.Errorf("mismatch forward link '%x' != '%x'",
				link.to, chain.links[i+1].from)
		}

		authority = authority.Apply(link.changeset)
	}

	return nil
}
