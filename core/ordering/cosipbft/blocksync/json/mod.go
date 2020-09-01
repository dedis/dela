package json

import (
	"encoding/json"

	"go.dedis.ch/dela/core/ordering/cosipbft/blocksync/types"
	otypes "go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	types.RegisterMessageFormat(serde.FormatJSON, msgFormat{})
}

// SyncMessageJSON is the JSON representation of a sync announcement.
type SyncMessageJSON struct {
	Chain json.RawMessage
}

// SyncRequestJSON is the JSON representation of a sync request.
type SyncRequestJSON struct {
	From uint64
}

// SyncReplyJSON is the JSON representation of a sync reply.
type SyncReplyJSON struct {
	Link json.RawMessage
}

// SyncAckJSON is the JSON representation of a sync acknowledgement.
type SyncAckJSON struct{}

// MessageJSON is the JSON representation of a sync message.
type MessageJSON struct {
	Message *SyncMessageJSON `json:",omitempty"`
	Request *SyncRequestJSON `json:",omitempty"`
	Reply   *SyncReplyJSON   `json:",omitempty"`
	Ack     *SyncAckJSON     `json:",omitempty"`
}

// MsgFormat is the format engine to encode and decode sync messages.
//
// - implements serde.FormatEngine
type msgFormat struct{}

// Encode implements serde.FormatEngine. It returns the JSON data of the message
// if appropriate, otherwise an error.
func (fmt msgFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	var m MessageJSON

	switch in := msg.(type) {
	case types.SyncMessage:
		chain, err := in.GetChain().Serialize(ctx)
		if err != nil {
			return nil, xerrors.Errorf("failed to encode chain: %v", err)
		}

		sm := SyncMessageJSON{
			Chain: chain,
		}

		m.Message = &sm
	case types.SyncRequest:
		req := SyncRequestJSON{
			From: in.GetFrom(),
		}

		m.Request = &req
	case types.SyncReply:
		link, err := in.GetLink().Serialize(ctx)
		if err != nil {
			return nil, xerrors.Errorf("link serialization failed: %v", err)
		}

		reply := SyncReplyJSON{
			Link: link,
		}

		m.Reply = &reply
	case types.SyncAck:
		m.Ack = &SyncAckJSON{}
	default:
		return nil, xerrors.Errorf("unsupported message '%T'", msg)
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("marshal failed: %v", err)
	}

	return data, nil
}

// Decode implements serde.FormatEngine. It returns the message associated to
// the data if appropriate, otherwise an error.
func (fmt msgFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := MessageJSON{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("unmarshal failed: %v", err)
	}

	if m.Message != nil {
		fac := ctx.GetFactory(types.ChainKey{})

		factory, ok := fac.(otypes.ChainFactory)
		if !ok {
			return nil, xerrors.Errorf("invalid chain factory '%T'", fac)
		}

		chain, err := factory.ChainOf(ctx, m.Message.Chain)
		if err != nil {
			return nil, xerrors.Errorf("failed to decode chain: %v", err)
		}

		return types.NewSyncMessage(chain), nil
	}

	if m.Request != nil {
		return types.NewSyncRequest(m.Request.From), nil
	}

	if m.Reply != nil {
		fac := ctx.GetFactory(types.LinkKey{})

		factory, ok := fac.(otypes.LinkFactory)
		if !ok {
			return nil, xerrors.Errorf("invalid link factory '%T'", fac)
		}

		link, err := factory.BlockLinkOf(ctx, m.Reply.Link)
		if err != nil {
			return nil, xerrors.Errorf("couldn't decode link: %v", err)
		}

		return types.NewSyncReply(link), nil
	}

	if m.Ack != nil {
		return types.NewSyncAck(), nil
	}

	return nil, xerrors.New("message is empty")
}
