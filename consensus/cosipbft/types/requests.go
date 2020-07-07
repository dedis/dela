package types

import (
	"go.dedis.ch/dela/consensus"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
)

var requestFormats = registry.NewSimpleRegistry()

// RegisterRequestFormat registers the engine for the provided format.
func RegisterRequestFormat(c serde.Format, f serde.FormatEngine) {
	requestFormats.Register(c, f)
}

// Prepare is the request sent at the beginning of the PBFT protocol.
//
// - implements serde.Message
type Prepare struct {
	message   serde.Message
	signature crypto.Signature
	chain     consensus.Chain
}

// NewPrepare creates a new prepare request.
func NewPrepare(msg serde.Message, sig crypto.Signature, chain consensus.Chain) Prepare {
	return Prepare{
		message:   msg,
		signature: sig,
		chain:     chain,
	}
}

// GetMessage returns the message.
func (p Prepare) GetMessage() serde.Message {
	return p.message
}

// GetSignature returns the signature.
func (p Prepare) GetSignature() crypto.Signature {
	return p.signature
}

// GetChain returns the chain.
func (p Prepare) GetChain() consensus.Chain {
	return p.chain
}

// Serialize implements serde.Messsage.
func (p Prepare) Serialize(ctx serde.Context) ([]byte, error) {
	format := requestFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, p)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode prepare: %v", err)
	}

	return data, nil
}

// Commit is the request sent for the last phase of the PBFT.
//
// - implements serde.Message
type Commit struct {
	to      Digest
	prepare crypto.Signature
}

// NewCommit creates a new commit request.
func NewCommit(to []byte, prepare crypto.Signature) Commit {
	commit := Commit{
		to:      to,
		prepare: prepare,
	}

	return commit
}

// GetTo returns the identifier for this commit.
func (c Commit) GetTo() []byte {
	return append([]byte{}, c.to...)
}

// GetPrepare returns the prepare phase signature.
func (c Commit) GetPrepare() crypto.Signature {
	return c.prepare
}

// Serialize implements serde.Message.
func (c Commit) Serialize(ctx serde.Context) ([]byte, error) {
	format := requestFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, c)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode commit: %v", err)
	}

	return data, nil
}

// Propagate is the final message sent to commit to a new proposal.
//
// -  implements serde.Message
type Propagate struct {
	to     []byte
	commit crypto.Signature
}

// NewPropagate creates a new propagate request.
func NewPropagate(to []byte, commit crypto.Signature) Propagate {
	return Propagate{
		to:     to,
		commit: commit,
	}
}

// GetTo returns the identifier for this request.
func (p Propagate) GetTo() []byte {
	return append([]byte{}, p.to...)
}

// GetCommit returns the commit phase signature.
func (p Propagate) GetCommit() crypto.Signature {
	return p.commit
}

// Serialize implements serde.Message.
func (p Propagate) Serialize(ctx serde.Context) ([]byte, error) {
	format := requestFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, p)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode propagate: %v", err)
	}

	return data, nil
}

// MsgKeyFac is the key for the message factory.
type MsgKeyFac struct{}

// SigKeyFac is the key for the signature factory.
type SigKeyFac struct{}

// CoSigKeyFac is the key for the collective signature factory.
type CoSigKeyFac struct{}

// ChainKeyFac is the key for the chain factory.
type ChainKeyFac struct{}

// RequestFactory is the factory to deserialize prepare and commit messages.
//
// - implements serde.Factory
type RequestFactory struct {
	msgFactory   serde.Factory
	sigFactory   crypto.SignatureFactory
	cosiFactory  crypto.SignatureFactory
	chainFactory consensus.ChainFactory
}

// NewRequestFactory creates a new request factory.
func NewRequestFactory(
	mf serde.Factory,
	sf, cosf crypto.SignatureFactory,
	cf consensus.ChainFactory) RequestFactory {

	return RequestFactory{
		msgFactory:   mf,
		sigFactory:   sf,
		cosiFactory:  cosf,
		chainFactory: cf,
	}
}

// Deserialize implements serde.Factory.
func (f RequestFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := requestFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, MsgKeyFac{}, f.msgFactory)
	ctx = serde.WithFactory(ctx, SigKeyFac{}, f.sigFactory)
	ctx = serde.WithFactory(ctx, CoSigKeyFac{}, f.cosiFactory)
	ctx = serde.WithFactory(ctx, ChainKeyFac{}, f.chainFactory)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode request: %v", err)
	}

	return msg, nil
}
