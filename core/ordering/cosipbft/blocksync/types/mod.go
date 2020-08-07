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

// SyncMessage is the announcement sent to the participants with the latest
// index of the leader.
//
// TODO: proof
//
// - implements serde.Message
type SyncMessage struct {
	latestIndex uint64
}

// NewSyncMessage creates a new announcement message.
func NewSyncMessage(index uint64) SyncMessage {
	return SyncMessage{
		latestIndex: index,
	}
}

// GetLatestIndex returns the latest index.
func (m SyncMessage) GetLatestIndex() uint64 {
	return m.latestIndex
}

// Serialize implements serde.Message. It returns the serialized data for this
// message.
func (m SyncMessage) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, m)
	if err != nil {
		return nil, xerrors.Errorf("encoding failed: %v", err)
	}

	return data, nil
}

// SyncRequest is a message to request missing blocks from a given index.
//
// - implements serde.Message
type SyncRequest struct {
	from uint64
}

// NewSyncRequest creates a new sync request.
func NewSyncRequest(from uint64) SyncRequest {
	return SyncRequest{
		from: from,
	}
}

// GetFrom returns the expected index of the first block when catching up.
func (m SyncRequest) GetFrom() uint64 {
	return m.from
}

// Serialize implements serde.Message. It returns the serialized data for this
// message.
func (m SyncRequest) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, m)
	if err != nil {
		return nil, xerrors.Errorf("encoding failed: %v", err)
	}

	return data, nil
}

// SyncReply is a message to send a block to a participant.
//
// - implements serde.Message
type SyncReply struct {
	link types.BlockLink
}

// NewSyncReply creates a new sync reply.
func NewSyncReply(link types.BlockLink) SyncReply {
	return SyncReply{
		link: link,
	}
}

// GetLink returns the link to a block to catch up.
func (m SyncReply) GetLink() types.BlockLink {
	return m.link
}

// Serialize implements serde.Message. It returns the serialized data for this
// message.
func (m SyncReply) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, m)
	if err != nil {
		return nil, xerrors.Errorf("encoding failed: %v", err)
	}

	return data, nil
}

// SyncAck is a message sent to confirm a hard synchronization.
//
// - implements serde.Message
type SyncAck struct{}

// NewSyncAck creates a new sync acknowledgement.
func NewSyncAck() SyncAck {
	return SyncAck{}
}

// Serialize implements serde.Message. It returns the serialized data for this
// message.
func (m SyncAck) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, m)
	if err != nil {
		return nil, xerrors.Errorf("encoding failed: %v", err)
	}

	return data, nil
}

// LinkKey is the key for the block link factory.
type LinkKey struct{}

// MessageFactory is a message factory for sync messages.
//
// - implements serde.Factory
type MessageFactory struct {
	linkFac types.BlockLinkFactory
}

// NewMessageFactory createsa new message factory.
func NewMessageFactory(fac types.BlockLinkFactory) MessageFactory {
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
