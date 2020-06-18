package cosi

import (
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serdeng"
	"go.dedis.ch/dela/serdeng/registry"
	"golang.org/x/xerrors"
)

var formats = registry.NewSimpleRegistry()

// Register registers the format for the given format name.
func Register(name serdeng.Codec, f serdeng.Format) {
	formats.Register(name, f)
}

// SignatureRequest is the message sent to require a signature from the other
// participants.
//
// - implements serde.Message
type SignatureRequest struct {
	Value serdeng.Message
}

// Serialize implements serde.Message. It serializes the message in the format
// supported by the context.
func (req SignatureRequest) Serialize(ctx serdeng.Context) ([]byte, error) {
	format := formats.Get(ctx.GetName())
	if format == nil {
		return nil, xerrors.Errorf("format '%s' is not supported", ctx.GetName())
	}

	data, err := format.Encode(ctx, req)
	if err != nil {
		return nil, xerrors.Errorf("format failed to encode: %v", err)
	}

	return data, nil
}

// SignatureResponse is the message sent by the participants.
//
// - implements serde.Message
type SignatureResponse struct {
	Signature crypto.Signature
}

// Serialize implements serde.Message. It serializes the message in the format
// supported by the context.
func (resp SignatureResponse) Serialize(ctx serdeng.Context) ([]byte, error) {
	format := formats.Get(ctx.GetName())
	if format == nil {
		return nil, xerrors.Errorf("format '%s' is not supported", ctx.GetName())
	}

	data, err := format.Encode(ctx, resp)
	if err != nil {
		return nil, xerrors.Errorf("format failed to encode: %v", err)
	}

	return data, nil
}

type MsgKey struct{}
type SigKey struct{}

// MessageFactory is the message factory for the flat collective signing RPC.
//
// - implements serde.Factory
type MessageFactory struct {
	msgFactory serdeng.Factory
	sigFactory crypto.SignatureFactory
}

// NewMessageFactory returns a new message factory that uses the message and
// signature factories.
func NewMessageFactory(msg serdeng.Factory, sig crypto.SignatureFactory) MessageFactory {
	return MessageFactory{
		msgFactory: msg,
		sigFactory: sig,
	}
}

// GetMessageFactory returns the message factory.
func (f MessageFactory) GetMessageFactory() serdeng.Factory {
	return f.msgFactory
}

// GetSignatureFactory returns the signature factory.
func (f MessageFactory) GetSignatureFactory() crypto.SignatureFactory {
	return f.sigFactory
}

// Deserialize implements serde.Factory.
func (f MessageFactory) Deserialize(ctx serdeng.Context, data []byte) (serdeng.Message, error) {
	format := formats.Get(ctx.GetName())
	if format == nil {
		return nil, xerrors.Errorf("format '%s' is not supported", ctx.GetName())
	}

	ctx = serdeng.WithFactory(ctx, MsgKey{}, f.msgFactory)
	ctx = serdeng.WithFactory(ctx, SigKey{}, f.sigFactory)

	m, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("format failed to decode: %v", err)
	}

	return m, nil
}
