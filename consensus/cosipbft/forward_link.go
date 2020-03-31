package cosipbft

import (
	"bytes"
	"crypto/sha256"
	"hash"

	"github.com/golang/protobuf/proto"
	any "github.com/golang/protobuf/ptypes/any"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"golang.org/x/xerrors"
)

// Digest is an alias for the bytes type for hash.
type Digest = []byte

// forwardLink is the cryptographic primitive to ensure a block is a successor
// of a previous one.
type forwardLink struct {
	hash Digest
	from Digest
	to   Digest
	// prepare is the signature of the combination of From and To to prove that
	// the nodes agreed on a valid forward link between the two blocks.
	prepare crypto.Signature
	// commit is the signature of the Prepare signature to prove that a
	// threshold of the nodes have committed the block.
	commit crypto.Signature
}

// Verify makes sure the signatures of the forward link are correct.
func (fl forwardLink) Verify(v crypto.Verifier) error {
	err := v.Verify(fl.hash, fl.prepare)
	if err != nil {
		return xerrors.Errorf("couldn't verify prepare signature: %w", err)
	}

	buffer, err := fl.prepare.MarshalBinary()
	if err != nil {
		return xerrors.Errorf("couldn't marshal the signature: %w", err)
	}

	fabric.Logger.Trace().Msgf("verifying commit %x", buffer)
	err = v.Verify(buffer, fl.commit)
	if err != nil {
		return xerrors.Errorf("couldn't verify commit signature: %w", err)
	}

	return nil
}

// Pack returns the protobuf message of the forward link.
func (fl forwardLink) Pack(enc encoding.ProtoMarshaler) (proto.Message, error) {
	pb := &ForwardLinkProto{
		From: fl.from,
		To:   fl.to,
	}

	var err error

	if fl.prepare != nil {
		pb.Prepare, err = enc.PackAny(fl.prepare)
		if err != nil {
			return nil, xerrors.Errorf("encoder: %v", err)
		}
	}

	if fl.commit != nil {
		pb.Commit, err = enc.PackAny(fl.commit)
		if err != nil {
			return nil, xerrors.Errorf("encoder: %v", err)
		}
	}

	return pb, nil
}

func (fl forwardLink) computeHash(h hash.Hash) ([]byte, error) {
	_, err := h.Write(fl.from)
	if err != nil {
		return nil, xerrors.Errorf("couldn't write 'from': %v", err)
	}
	_, err = h.Write(fl.to)
	if err != nil {
		return nil, xerrors.Errorf("couldn't write 'to': %v", err)
	}

	return h.Sum(nil), nil
}

type forwardLinkChain struct {
	links []forwardLink
}

// GetLastHash implements consensus.Chain. It returns the last hash of the
// chain.
func (c forwardLinkChain) GetLastHash() []byte {
	if len(c.links) == 0 {
		return nil
	}

	return c.links[len(c.links)-1].to
}

// Verify follows the chain from the beginning and makes sure that the forward
// links are correct and that they point to the correct targets.
func (c forwardLinkChain) Verify(verifier crypto.Verifier) error {
	if len(c.links) == 0 {
		return xerrors.New("chain is empty")
	}

	lastIndex := len(c.links) - 1

	for i, link := range c.links {
		err := link.Verify(verifier)
		if err != nil {
			return xerrors.Errorf("couldn't verify link %d: %w", i, err)
		}

		if i != lastIndex && !bytes.Equal(link.to, c.links[i+1].from) {
			return xerrors.Errorf("mismatch forward link '%x' != '%x'",
				link.to, c.links[i+1].from)
		}
	}

	return nil
}

// Pack returs the protobuf message for the chain.
func (c forwardLinkChain) Pack(enc encoding.ProtoMarshaler) (proto.Message, error) {
	pb := &ChainProto{
		Links: make([]*ForwardLinkProto, len(c.links)),
	}

	for i, link := range c.links {
		packed, err := enc.Pack(link)
		if err != nil {
			return nil, xerrors.Errorf("encoder: %v", err)
		}

		pb.Links[i] = packed.(*ForwardLinkProto)
	}

	return pb, nil
}

type sha256Factory struct{}

func (f sha256Factory) New() hash.Hash {
	return sha256.New()
}

// chainFactory is an implementation of the chainFactory interface
// for forward links.
type chainFactory struct {
	signatureFactory crypto.SignatureFactory
	hashFactory      crypto.HashFactory
	encoder          encoding.ProtoMarshaler
}

// newChainFactory returns a new instance of a seal factory that will create
// forward links for appropriate protobuf messages and return an error
// otherwise.
func newChainFactory(f crypto.SignatureFactory) *chainFactory {
	return &chainFactory{
		signatureFactory: f,
		hashFactory:      sha256Factory{},
		encoder:          encoding.NewProtoEncoder(),
	}
}

func (f *chainFactory) decodeForwardLink(pb proto.Message) (forwardLink, error) {
	var fl forwardLink
	var src *ForwardLinkProto
	switch msg := pb.(type) {
	case *any.Any:
		src = &ForwardLinkProto{}

		err := f.encoder.UnmarshalAny(msg, src)
		if err != nil {
			return fl, xerrors.Errorf("encoder: %v", err)
		}
	case *ForwardLinkProto:
		src = msg
	default:
		return fl, xerrors.Errorf("unknown message type: %T", msg)
	}

	fl = forwardLink{
		from: src.GetFrom(),
		to:   src.GetTo(),
	}

	if src.GetPrepare() != nil {
		sig, err := f.signatureFactory.FromProto(src.GetPrepare())
		if err != nil {
			return fl, encoding.NewDecodingError("prepare signature", err)
		}

		fl.prepare = sig
	}

	if src.GetCommit() != nil {
		sig, err := f.signatureFactory.FromProto(src.GetCommit())
		if err != nil {
			return fl, encoding.NewDecodingError("commit signature", err)
		}

		fl.commit = sig
	}

	hash, err := fl.computeHash(f.hashFactory.New())
	if err != nil {
		return fl, xerrors.Errorf("couldn't hash the forward link: %v", err)
	}

	fl.hash = hash

	return fl, nil
}

// FromProto returns a chain from a protobuf message.
func (f *chainFactory) FromProto(pb proto.Message) (consensus.Chain, error) {
	var msg *ChainProto
	switch in := pb.(type) {
	case *any.Any:
		msg = &ChainProto{}

		err := f.encoder.UnmarshalAny(in, msg)
		if err != nil {
			return nil, xerrors.Errorf("encoder: oops")
		}
	case *ChainProto:
		msg = in
	default:
		return nil, xerrors.Errorf("message type not supported: %T", in)
	}

	chain := forwardLinkChain{
		links: make([]forwardLink, len(msg.GetLinks())),
	}
	for i, plink := range msg.GetLinks() {
		link, err := f.decodeForwardLink(plink)
		if err != nil {
			return nil, err
		}

		chain.links[i] = link
	}

	return chain, nil
}
