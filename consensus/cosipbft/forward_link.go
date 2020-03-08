package cosipbft

import (
	"bytes"
	"crypto/sha256"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	any "github.com/golang/protobuf/ptypes/any"
	"go.dedis.ch/fabric"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"golang.org/x/xerrors"
)

var protoenc encoding.ProtoMarshaler = encoding.NewProtoEncoder()

// Digest is an alias for the bytes type for hash.
type Digest = []byte

// forwardLink is the cryptographic primitive to ensure a block is a successor
// of a previous one.
type forwardLink struct {
	hash Digest
	from Digest
	to   Digest
	// prepare signs the combination of From and To to prove that the nodes
	// agreed on a valid forward link between the two blocks.
	prepare crypto.Signature
	// commit signs the Prepare signature to prove that a threshold of the
	// nodes have committed the block.
	commit crypto.Signature
}

// Verify makes sure the signatures of the forward link are correct.
func (fl forwardLink) Verify(v crypto.Verifier, pubkeys []crypto.PublicKey) error {
	err := v.Verify(pubkeys, fl.hash, fl.prepare)
	if err != nil {
		return xerrors.Errorf("couldn't verify prepare signature: %w", err)
	}

	buffer, err := fl.prepare.MarshalBinary()
	if err != nil {
		return xerrors.Errorf("couldn't marshal the signature: %w", err)
	}

	fabric.Logger.Trace().Msgf("verifying commit %x", buffer)
	err = v.Verify(pubkeys, buffer, fl.commit)
	if err != nil {
		return xerrors.Errorf("couldn't verify commit signature: %w", err)
	}

	return nil
}

func encodeSignature(sig crypto.Signature) (*any.Any, error) {
	packed, err := sig.Pack()
	if err != nil {
		return nil, xerrors.Errorf("couldn't pack: %v", err)
	}

	packedAny, err := protoenc.MarshalAny(packed)
	if err != nil {
		return nil, encoding.NewAnyEncodingError(packed, err)
	}

	return packedAny, nil
}

// Pack returns the protobuf message of the forward link.
func (fl forwardLink) Pack() (proto.Message, error) {
	seal := &ForwardLinkProto{
		From: fl.from,
		To:   fl.to,
	}

	var err error

	if fl.prepare != nil {
		seal.Prepare, err = encodeSignature(fl.prepare)
		if err != nil {
			return nil, encoding.NewEncodingError("prepare", err)
		}
	}

	if fl.commit != nil {
		seal.Commit, err = encodeSignature(fl.commit)
		if err != nil {
			return nil, encoding.NewEncodingError("commit", err)
		}
	}

	return seal, nil
}

func (fl forwardLink) computeHash() ([]byte, error) {
	// TODO: optional algorithm + errors
	h := sha256.New()

	h.Write(fl.from)
	h.Write(fl.to)

	return h.Sum(nil), nil
}

type forwardLinkChain struct {
	links []forwardLink
}

// Verify follows the chain from the beginning and makes sure that the forward
// links are correct and that they point to the correct targets.
func (c forwardLinkChain) Verify(verifier crypto.Verifier, pubkeys []crypto.PublicKey) error {
	if len(c.links) == 0 {
		return xerrors.New("chain is empty")
	}

	lastIndex := len(c.links) - 1

	for i, link := range c.links {
		err := link.Verify(verifier, pubkeys)
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
func (c forwardLinkChain) Pack() (proto.Message, error) {
	pb := &ChainProto{
		Links: make([]*ForwardLinkProto, len(c.links)),
	}

	for i, link := range c.links {
		packed, err := link.Pack()
		if err != nil {
			return nil, encoding.NewEncodingError("forward link", err)
		}

		pb.Links[i] = packed.(*ForwardLinkProto)
	}

	return pb, nil
}

// ChainFactory is an implementation of the ChainFactory interface for forward links.
type ChainFactory struct {
	verifier crypto.Verifier
}

// NewChainFactory returns a new insance of a seal factory that will create
// forward links for appropriate protobuf messages and return an error
// otherwise.
func NewChainFactory(verifier crypto.Verifier) *ChainFactory {
	return &ChainFactory{verifier: verifier}
}

func (f *ChainFactory) decodeSignature(pb proto.Message) (crypto.Signature, error) {
	sig, err := f.verifier.GetSignatureFactory().FromProto(pb)
	if err != nil {
		return nil, err
	}

	return sig, nil
}

func (f *ChainFactory) decodeLink(pb proto.Message) (*forwardLink, error) {
	var src *ForwardLinkProto
	switch msg := pb.(type) {
	case *any.Any:
		src = &ForwardLinkProto{}

		err := ptypes.UnmarshalAny(msg, src)
		if err != nil {
			return nil, err
		}
	case *ForwardLinkProto:
		src = msg
	default:
		return nil, xerrors.New("unknown message type")
	}

	fl := forwardLink{
		from: src.GetFrom(),
		to:   src.GetTo(),
	}

	if src.GetPrepare() != nil {
		sig, err := f.decodeSignature(src.GetPrepare())
		if err != nil {
			return nil, encoding.NewDecodingError("prepare signature", err)
		}

		fl.prepare = sig
	}

	if src.GetCommit() != nil {
		sig, err := f.decodeSignature(src.GetCommit())
		if err != nil {
			return nil, encoding.NewDecodingError("commit signature", err)
		}

		fl.commit = sig
	}

	hash, err := fl.computeHash()
	if err != nil {
		return nil, xerrors.Errorf("couldn't hash the forward link: %v", err)
	}

	fl.hash = hash

	return &fl, nil
}

// FromProto returns a chain from a protobuf message.
func (f *ChainFactory) FromProto(pb proto.Message) (consensus.Chain, error) {
	var msg *ChainProto
	switch in := pb.(type) {
	case *any.Any:
		msg = &ChainProto{}

		err := ptypes.UnmarshalAny(in, msg)
		if err != nil {
			return nil, encoding.NewAnyDecodingError(msg, err)
		}
	case *ChainProto:
		msg = in
	default:
		return nil, xerrors.New("message type not supported")
	}

	chain := forwardLinkChain{
		links: make([]forwardLink, len(msg.GetLinks())),
	}
	for i, plink := range msg.GetLinks() {
		link, err := f.decodeLink(plink)
		if err != nil {
			return nil, encoding.NewDecodingError("forward link", err)
		}

		chain.links[i] = *link
	}

	return chain, nil
}
