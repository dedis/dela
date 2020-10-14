//
// Documentation Last Review: 05.10.2020
//

package cosi

import (
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
)

var msgFormats = registry.NewSimpleRegistry()

// RegisterMessageFormat registers the format for the given format name.
func RegisterMessageFormat(name serde.Format, f serde.FormatEngine) {
	msgFormats.Register(name, f)
}

// SignatureRequest is the message sent to require a signature from the other
// participants.
//
// - implements serde.Message
type SignatureRequest struct {
	Value serde.Message
}

// Serialize implements serde.Message. It looks up the format and returns the
// serialized data if appropriate, otherwise an error.
func (req SignatureRequest) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, req)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode request: %v", err)
	}

	return data, nil
}

// SignatureResponse is the message sent by the participants.
//
// - implements serde.Message
type SignatureResponse struct {
	Signature crypto.Signature
}

// Serialize implements serde.Message. It looks up the format and returns the
// serialized data if appropriate, otherwise an error.
func (resp SignatureResponse) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, resp)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode response: %v", err)
	}

	return data, nil
}

// MsgKey is the key of the message factory.
type MsgKey struct{}

// SigKey is the key of the signature factory.
type SigKey struct{}

// MessageFactory is the message factory for the flat collective signing RPC.
//
// - implements serde.Factory
type MessageFactory struct {
	msgFactory serde.Factory
	sigFactory crypto.SignatureFactory
}

// NewMessageFactory returns a new message factory that uses the message and
// signature factories.
func NewMessageFactory(msg serde.Factory, sig crypto.SignatureFactory) MessageFactory {
	return MessageFactory{
		msgFactory: msg,
		sigFactory: sig,
	}
}

// Deserialize implements serde.Factory.
func (f MessageFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := msgFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, MsgKey{}, f.msgFactory)
	ctx = serde.WithFactory(ctx, SigKey{}, f.sigFactory)

	m, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode message: %v", err)
	}

	return m, nil
}
