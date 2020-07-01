package types

import (
	"bytes"
	"io"

	"go.dedis.ch/dela/consensus"
	"go.dedis.ch/dela/consensus/viewchange"
	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
)

var (
	linkFormats  = registry.NewSimpleRegistry()
	chainFormats = registry.NewSimpleRegistry()
)

func RegisterForwardLinkFormat(c serde.Codec, f serde.Format) {
	linkFormats.Register(c, f)
}

func RegisterChainFormat(c serde.Codec, f serde.Format) {
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

type ForwardLinkTemplate struct {
	ForwardLink
	factory crypto.HashFactory
}

type ForwardLinkOption func(*ForwardLinkTemplate)

func WithPrepare(s crypto.Signature) ForwardLinkOption {
	return func(link *ForwardLinkTemplate) {
		link.prepare = s
	}
}

func WithCommit(s crypto.Signature) ForwardLinkOption {
	return func(link *ForwardLinkTemplate) {
		link.commit = s
	}
}

func WithChangeSet(cs viewchange.ChangeSet) ForwardLinkOption {
	return func(link *ForwardLinkTemplate) {
		link.changeset = cs
	}
}

func WithHashFactory(f crypto.HashFactory) ForwardLinkOption {
	return func(link *ForwardLinkTemplate) {
		link.factory = f
	}
}

func NewForwardLink(from, to Digest, opts ...ForwardLinkOption) (ForwardLink, error) {
	tmpl := &ForwardLinkTemplate{
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
		return ForwardLink{}, err
	}

	tmpl.hash = h.Sum(nil)

	return tmpl.ForwardLink, nil
}

func (fl ForwardLink) GetFrom() []byte {
	return append([]byte{}, fl.from...)
}

func (fl ForwardLink) GetTo() []byte {
	return append([]byte{}, fl.to...)
}

func (fl ForwardLink) GetFingerprint() []byte {
	return fl.hash
}

func (fl ForwardLink) GetPrepareSignature() crypto.Signature {
	return fl.prepare
}

func (fl ForwardLink) GetCommitSignature() crypto.Signature {
	return fl.commit
}

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
	format := linkFormats.Get(ctx.GetName())

	data, err := format.Encode(ctx, fl)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode link: %v", err)
	}

	return data, nil
}

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

func NewChain(links ...ForwardLink) Chain {
	return Chain{links: links}
}

func (c Chain) Len() int {
	return len(c.links)
}

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
	format := chainFormats.Get(ctx.GetName())

	data, err := format.Encode(ctx, c)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode chain: %v", err)
	}

	return data, nil
}

type ChangeSetKey struct{}

// ChainFactory is an implementation of the chainFactory interface for forward
// links.
//
// - implements serde.Factory
type ChainFactory struct {
	sigFactory      crypto.SignatureFactory
	csFactory       viewchange.ChangeSetFactory
	viewchange      viewchange.ViewChange
	verifierFactory crypto.VerifierFactory
}

type ChainFactoryOption func(*ChainFactory)

func WithCoSi(c cosi.CollectiveSigning) ChainFactoryOption {
	return func(f *ChainFactory) {
		f.sigFactory = c.GetSignatureFactory()
		f.verifierFactory = c.GetVerifierFactory()
	}
}

func WithViewChange(vc viewchange.ViewChange) ChainFactoryOption {
	return func(f *ChainFactory) {
		f.viewchange = vc
		f.csFactory = vc.GetChangeSetFactory()
	}
}

func WithSignatureFactory(sf crypto.SignatureFactory) ChainFactoryOption {
	return func(f *ChainFactory) {
		f.sigFactory = sf
	}
}

func WithChangeSetFactory(csf viewchange.ChangeSetFactory) ChainFactoryOption {
	return func(f *ChainFactory) {
		f.csFactory = csf
	}
}

// NewChainFactory returns a new instance of a seal factory that will create
// forward links for appropriate protobuf messages and return an error
// otherwise.
func NewChainFactory(opts ...ChainFactoryOption) ChainFactory {
	f := ChainFactory{}

	for _, opt := range opts {
		opt(&f)
	}

	// TODO: check missing component

	return f
}

func (f ChainFactory) GetSignatureFactory() crypto.SignatureFactory {
	return f.sigFactory
}

func (f ChainFactory) GetChangeSetFactory() viewchange.ChangeSetFactory {
	return f.csFactory
}

// Deserialize implements serde.Factory.
func (f ChainFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := chainFormats.Get(ctx.GetName())

	ctx = serde.WithFactory(ctx, CoSigKey{}, f.sigFactory)
	ctx = serde.WithFactory(ctx, ChangeSetKey{}, f.csFactory)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, err
	}

	chain, ok := msg.(Chain)
	if !ok {
		return nil, xerrors.New("invalid chain")
	}

	err = f.verify(chain)
	if err != nil {
		return nil, xerrors.Errorf("couldn't verify the chain: %v", err)
	}

	return chain, nil
}

func (f ChainFactory) ChainOf(ctx serde.Context, data []byte) (consensus.Chain, error) {
	chain, err := f.Deserialize(ctx, data)
	if err != nil {
		return nil, err
	}

	return chain.(consensus.Chain), nil
}

func (f ChainFactory) verify(chain Chain) error {
	if len(chain.links) == 0 {
		return nil
	}

	authority, err := f.viewchange.GetAuthority(0)
	if err != nil {
		return err
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
