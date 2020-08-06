package types

import (
	"go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
)

var msgFormats = registry.NewSimpleRegistry()

func RegisterMessageFormat(f serde.Format, e serde.FormatEngine) {
	msgFormats.Register(f, e)
}

type SyncMessage struct {
	latestIndex uint64
}

func NewSyncMessage(index uint64) SyncMessage {
	return SyncMessage{
		latestIndex: index,
	}
}

func (m SyncMessage) GetLatestIndex() uint64 {
	return m.latestIndex
}

func (m SyncMessage) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, m)
	if err != nil {
		return nil, xerrors.Errorf("encoding failed: %v", err)
	}

	return data, nil
}

type SyncRequest struct {
	from uint64
}

func NewSyncRequest(from uint64) SyncRequest {
	return SyncRequest{
		from: from,
	}
}

func (m SyncRequest) GetFrom() uint64 {
	return m.from
}

func (m SyncRequest) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, m)
	if err != nil {
		return nil, xerrors.Errorf("encoding failed: %v", err)
	}

	return data, nil
}

type SyncReply struct {
	link types.BlockLink
}

func NewSyncReply(link types.BlockLink) SyncReply {
	return SyncReply{
		link: link,
	}
}

func (m SyncReply) GetLink() types.BlockLink {
	return m.link
}

func (m SyncReply) Serialize(ctx serde.Context) ([]byte, error) {
	format := msgFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, m)
	if err != nil {
		return nil, xerrors.Errorf("encoding failed: %v", err)
	}

	return data, nil
}

type LinkKey struct{}

type MessageFactory struct {
	linkFac serde.Factory
}

func NewMessageFactory(fac serde.Factory) MessageFactory {
	return MessageFactory{
		linkFac: fac,
	}
}

func (fac MessageFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := msgFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, LinkKey{}, fac.linkFac)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("decoding failed: %v", err)
	}

	return msg, nil
}
