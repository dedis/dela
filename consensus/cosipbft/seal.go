package cosipbft

import (
	"crypto/sha256"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	any "github.com/golang/protobuf/ptypes/any"
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

func (fl forwardLink) GetFrom() Digest {
	return fl.from
}

func (fl forwardLink) GetTo() Digest {
	return fl.to
}

// Verify makes sure the signatures of the forward link are correct.
func (fl forwardLink) Verify(v crypto.Verifier, pubkeys []crypto.PublicKey) error {
	err := v.Verify(pubkeys, fl.hash, fl.prepare)
	if err != nil {
		return xerrors.Errorf("couldn't verify the prepare signature: %w", err)
	}

	buffer, err := fl.prepare.MarshalBinary()
	if err != nil {
		return xerrors.Errorf("couldn't marshal the signature: %w", err)
	}

	err = v.Verify(pubkeys, buffer, fl.commit)
	if err != nil {
		return xerrors.Errorf("coudln't verify the commit signature: %w", err)
	}

	return nil
}

// Pack returns the protobuf message of the forward link.
func (fl forwardLink) Pack() (proto.Message, error) {
	seal := &ForwardLinkProto{
		From: fl.from,
		To:   fl.to,
	}

	if fl.prepare != nil {
		prepare, err := fl.prepare.Pack()
		if err != nil {
			return nil, encoding.NewEncodingError("prepare", err)
		}

		prepareAny, err := protoenc.MarshalAny(prepare)
		if err != nil {
			return nil, encoding.NewAnyEncodingError(prepare, err)
		}

		seal.Prepare = prepareAny
	}

	if fl.commit != nil {
		commit, err := fl.commit.Pack()
		if err != nil {
			return nil, encoding.NewEncodingError("commit", err)
		}

		commitAny, err := protoenc.MarshalAny(commit)
		if err != nil {
			return nil, encoding.NewAnyEncodingError(commit, err)
		}

		seal.Commit = commitAny
	}

	return seal, nil
}

func (fl forwardLink) computeHash() ([]byte, error) {
	h := sha256.New()

	h.Write(fl.from)
	h.Write(fl.to)

	return h.Sum(nil), nil
}

// ChainFactory is an implementation of the ChainFactory interface for forward links.
type ChainFactory struct {
	verifier crypto.Verifier
}

// NewSealFactory returns a new insance of a seal factory that will create
// forward links for appropriate protobuf messages and return an error
// otherwise.
func NewSealFactory(verifier crypto.Verifier) *ChainFactory {
	return &ChainFactory{verifier: verifier}
}

// FromProto returns an instance of a chain from appropriate messages.
func (f *ChainFactory) FromProto(pb proto.Message) (consensus.Chain, error) {
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
		sig, err := f.verifier.GetSignatureFactory().FromProto(src.GetPrepare())
		if err != nil {
			return nil, encoding.NewDecodingError("prepare signature", err)
		}

		fl.prepare = sig
	}

	if src.GetCommit() != nil {
		sig, err := f.verifier.GetSignatureFactory().FromProto(src.GetCommit())
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

	return fl, nil
}
