package json

import (
	"encoding/json"

	"go.dedis.ch/dela/core/ordering/cosipbft/blocksync/types"
	cosipbft "go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	types.RegisterMessageFormat(serde.FormatJSON, msgFormat{})
}

type SyncMessageJSON struct {
	LatestIndex uint64
}

type SyncRequestJSON struct {
	From uint64
}

type SyncReplyJSON struct {
	Link json.RawMessage
}

type MessageJSON struct {
	Message *SyncMessageJSON
	Request *SyncRequestJSON
	Reply   *SyncReplyJSON
}

type msgFormat struct{}

func (fmt msgFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	var m MessageJSON

	switch in := msg.(type) {
	case types.SyncMessage:
		sm := SyncMessageJSON{
			LatestIndex: in.GetLatestIndex(),
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
			return nil, err
		}

		reply := SyncReplyJSON{
			Link: link,
		}

		m.Reply = &reply
	default:
		return nil, xerrors.Errorf("unsupported message '%T'", msg)
	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (fmt msgFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := MessageJSON{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}

	if m.Message != nil {
		return types.NewSyncMessage(m.Message.LatestIndex), nil
	}

	if m.Request != nil {
		return types.NewSyncRequest(m.Request.From), nil
	}

	if m.Reply != nil {
		fac := ctx.GetFactory(types.LinkKey{})

		msg, err := fac.Deserialize(ctx, m.Reply.Link)
		if err != nil {
			return nil, err
		}

		link, ok := msg.(cosipbft.BlockLink)
		if !ok {
			return nil, xerrors.Errorf("invalid block link '%T'", msg)
		}

		return types.NewSyncReply(link), nil
	}

	return nil, xerrors.New("message is empty")
}
