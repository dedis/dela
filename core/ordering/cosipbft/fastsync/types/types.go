package types

import (
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
)

var msgFormats = registry.NewSimpleRegistry()

// RegisterMessageFormat registers the engine for the given format.
func RegisterMessageFormat(f serde.Format, e serde.FormatEngine) {
	msgFormats.Register(f, e)
}

// RequestCatchupMessage is sent by a node which wants to catch up to the latest
// block.
type RequestCatchupMessage struct {
	splitMessageSize uint64
	latest           uint64
}

// NewRequestCatchupMessage creates a RequestCatchupMessage
func NewRequestCatchupMessage(splitMessageSize, latest uint64) RequestCatchupMessage {
	return RequestCatchupMessage{splitMessageSize: splitMessageSize, latest: latest}
}

// GetLatest returns the latest index requested by the sender.
func (m RequestCatchupMessage) GetLatest() uint64 {
	return m.latest
}

// GetSplitMessageSize returns the size at which a message should be split.
func (m RequestCatchupMessage) GetSplitMessageSize() uint64 {
	return m.splitMessageSize
}

// Serialize implements serde.Message. It returns the serialized data for this
// message.
func (m RequestCatchupMessage) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, m)
	if err != nil {
		return nil, xerrors.Errorf("encoding failed: %v", err)
	}

	return data, nil
}

// CatchupMessage returns all the blocks, not just the links, so that the
// node can re-create the correct global state.
// 'splitMessage' is true if the node knows about more nodes.
type CatchupMessage struct {
	splitMessage bool
	blockLinks   []types.BlockLink
}

// NewCatchupMessage creates a reply to RequestLatestMessage.
func NewCatchupMessage(splitMessage bool, blockLinks []types.BlockLink) CatchupMessage {
	return CatchupMessage{splitMessage: splitMessage, blockLinks: blockLinks}
}

// GetBlockLinks returns the BlockLinks of the catchup.
func (m CatchupMessage) GetBlockLinks() []types.BlockLink {
	return m.blockLinks
}

// GetSplitMessage returns if the sending node has more blocks.
func (m CatchupMessage) GetSplitMessage() bool {
	return m.splitMessage
}

// Serialize implements serde.Message. It returns the serialized data for this
// message.
func (m CatchupMessage) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, m)
	if err != nil {
		return nil, xerrors.Errorf("encoding failed: %v", err)
	}

	return data, nil
}

// LinkKey is the key of the block link factory.
type LinkKey struct{}

// MessageFactory is a message factory for sync messages.
//
// - implements serde.Factory
type MessageFactory struct {
	linkFac types.LinkFactory
}

// NewMessageFactory creates new message factory.
func NewMessageFactory(fac types.LinkFactory) MessageFactory {
	return MessageFactory{
		linkFac: fac,
	}
}

// Deserialize implements serde.Factory. It returns the message associated to
// the data if appropriate, otherwise an error.
func (fac MessageFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := msgFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, LinkKey{}, fac.linkFac)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("decoding failed: %v", err)
	}

	return msg, nil
}
