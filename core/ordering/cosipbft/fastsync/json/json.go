package json

import (
	"encoding/json"

	"go.dedis.ch/dela/core/ordering/cosipbft/fastsync/types"
	otypes "go.dedis.ch/dela/core/ordering/cosipbft/types"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	types.RegisterMessageFormat(serde.FormatJSON, msgFormat{})
}

// RequestCatchupMessageJSON is the JSON representation of a request catchup
// message.
type RequestCatchupMessageJSON struct {
	SplitMessageSize uint64
	Latest           uint64
}

// CatchupMessageJSON is the JSON representation of all the new BlockLinks.
type CatchupMessageJSON struct {
	SplitMessage bool
	BlockLinks   []json.RawMessage
}

// MessageJSON is the JSON representation of a sync message.
type MessageJSON struct {
	Request *RequestCatchupMessageJSON `json:",omitempty"`
	Catchup *CatchupMessageJSON        `json:",omitempty"`
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
	case types.RequestCatchupMessage:
		request := RequestCatchupMessageJSON{
			SplitMessageSize: in.GetSplitMessageSize(),
			Latest:           in.GetLatest(),
		}

		m.Request = &request
	case types.CatchupMessage:
		bls := in.GetBlockLinks()
		catchup := CatchupMessageJSON{
			SplitMessage: in.GetSplitMessage(),
			BlockLinks:   make([]json.RawMessage, len(bls)),
		}

		for i, bl := range bls {
			blBuf, err := bl.Serialize(ctx)
			if err != nil {
				return nil, xerrors.Errorf("failed to encode blocklink: %v", err)
			}
			catchup.BlockLinks[i] = blBuf
		}

		m.Catchup = &catchup
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

	if m.Request != nil {
		return types.NewRequestCatchupMessage(m.Request.SplitMessageSize, m.Request.Latest), nil
	}

	if m.Catchup != nil {
		fac := ctx.GetFactory(types.LinkKey{})

		factory, ok := fac.(otypes.LinkFactory)
		if !ok {
			return nil, xerrors.Errorf("invalid link factory '%T'", fac)
		}

		var blockLinks = make([]otypes.BlockLink, len(m.Catchup.BlockLinks))
		for i, blBuf := range m.Catchup.BlockLinks {
			blockLinks[i], err = factory.BlockLinkOf(ctx, blBuf)
			if err != nil {
				return nil, xerrors.Errorf("failed to decode blockLink: %v", err)
			}
		}

		return types.NewCatchupMessage(m.Catchup.SplitMessage, blockLinks), nil
	}

	return nil, xerrors.New("message is empty")
}
