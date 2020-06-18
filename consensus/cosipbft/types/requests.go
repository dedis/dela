package types

import (
	"go.dedis.ch/dela/consensus"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serdeng"
	"go.dedis.ch/dela/serdeng/registry"
)

var requestFormats = registry.NewSimpleRegistry()

func RegisterRequestFormat(c serdeng.Codec, f serdeng.Format) {
	requestFormats.Register(c, f)
}

// Prepare is the request sent at the beginning of the PBFT protocol.
//
// - implements serde.Message
type Prepare struct {
	message   serdeng.Message
	signature crypto.Signature
	chain     consensus.Chain
}

func NewPrepare(msg serdeng.Message, sig crypto.Signature, chain consensus.Chain) Prepare {
	return Prepare{
		message:   msg,
		signature: sig,
		chain:     chain,
	}
}

func (p Prepare) GetMessage() serdeng.Message {
	return p.message
}

func (p Prepare) GetSignature() crypto.Signature {
	return p.signature
}

func (p Prepare) GetChain() consensus.Chain {
	return p.chain
}

// Serialize implements serde.Messsage.
func (p Prepare) Serialize(ctx serdeng.Context) ([]byte, error) {
	format := requestFormats.Get(ctx.GetName())

	data, err := format.Encode(ctx, p)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// Commit is the request sent for the last phase of the PBFT.
//
// - implements serde.Message
type Commit struct {
	serde.UnimplementedMessage

	to      Digest
	prepare crypto.Signature
}

func NewCommit(to []byte, prepare crypto.Signature) Commit {
	commit := Commit{
		to:      to,
		prepare: prepare,
	}

	return commit
}

func (c Commit) GetTo() []byte {
	return append([]byte{}, c.to...)
}

func (c Commit) GetPrepare() crypto.Signature {
	return c.prepare
}

// Serialize implements serde.Message.
func (c Commit) Serialize(ctx serdeng.Context) ([]byte, error) {
	format := requestFormats.Get(ctx.GetName())

	data, err := format.Encode(ctx, c)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// Propagate is the final message sent to commit to a new proposal.
//
// -  implements serde.Message
type Propagate struct {
	serde.UnimplementedMessage

	to     []byte
	commit crypto.Signature
}

func NewPropagate(to []byte, commit crypto.Signature) Propagate {
	return Propagate{
		to:     to,
		commit: commit,
	}
}

func (p Propagate) GetTo() []byte {
	return append([]byte{}, p.to...)
}

func (p Propagate) GetCommit() crypto.Signature {
	return p.commit
}

// Serialize implements serde.Message.
func (p Propagate) Serialize(ctx serdeng.Context) ([]byte, error) {
	format := requestFormats.Get(ctx.GetName())

	data, err := format.Encode(ctx, p)
	if err != nil {
		return nil, err
	}

	return data, nil
}

type MsgKey struct{}
type SigKey struct{}
type CoSigKey struct{}
type ChainKey struct{}

// RequestFactory is the factory to deserialize prepare and commit messages.
//
// - implements serde.Factory
type RequestFactory struct {
	msgFactory   serdeng.Factory
	sigFactory   crypto.SignatureFactory
	cosiFactory  crypto.SignatureFactory
	chainFactory consensus.ChainFactory
}

func NewRequestFactory(mf serdeng.Factory, sf, cosf crypto.SignatureFactory, cf consensus.ChainFactory) RequestFactory {
	return RequestFactory{
		msgFactory:   mf,
		sigFactory:   sf,
		cosiFactory:  cosf,
		chainFactory: cf,
	}
}

func (f RequestFactory) GetMessageFactory() serdeng.Factory {
	return f.msgFactory
}

func (f RequestFactory) GetSignatureFactory() crypto.SignatureFactory {
	return f.sigFactory
}

func (f RequestFactory) GetCoSignatureFactory() crypto.SignatureFactory {
	return f.cosiFactory
}

func (f RequestFactory) GetChainFactory() consensus.ChainFactory {
	return f.chainFactory
}

// Deserialize implements serde.Factory.
func (f RequestFactory) Deserialize(ctx serdeng.Context, data []byte) (serdeng.Message, error) {
	format := requestFormats.Get(ctx.GetName())

	ctx = serdeng.WithFactory(ctx, MsgKey{}, f.msgFactory)
	ctx = serdeng.WithFactory(ctx, SigKey{}, f.sigFactory)
	ctx = serdeng.WithFactory(ctx, CoSigKey{}, f.cosiFactory)
	ctx = serdeng.WithFactory(ctx, ChainKey{}, f.chainFactory)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, err
	}

	return msg, nil
}
