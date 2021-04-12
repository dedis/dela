package json

import (
	"go.dedis.ch/dela/contracts/evoting/types"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	types.RegisterMessageFormat(serde.FormatJSON, newMsgFormat())
}

type Message struct {
	Election      *types.Election      `json:",omitempty"`
	Ballot        *types.Ballot        `json:",omitempty"`
	Configuration *types.Configuration `json:",omitempty"`
	Subject       *types.Subject       `json:",omitempty"`
	Select        *types.Select        `json:",omitempty"`
	Rank          *types.Rank          `json:",omitempty"`
	Text          *types.Text          `json:",omitempty"`
}

// MsgFormat is the engine to encode and decode dkg messages in JSON format.
//
// - implements serde.FormatEngine
type msgFormat struct {
}

func newMsgFormat() msgFormat {
	return msgFormat{
	}
}

// Encode implements serde.FormatEngine. It returns the serialized data for the
// message in JSON format.
func (f msgFormat) Encode(ctx serde.Context, message serde.Message) ([]byte, error) {
	var m Message

	switch in := message.(type) {

	case types.Election:
		m = Message{Election: &in}

	case types.Ballot:
		m = Message{Ballot: &in}

	case types.Configuration:
		m = Message{Configuration: &in}

	case types.Subject:
		m = Message{Subject: &in}

	case types.Select:
		m = Message{Select: &in}

	case types.Rank:
		m = Message{Rank: &in}

	case types.Text:
		m = Message{Text: &in}

	default:
		return nil, xerrors.Errorf("unsupported message of type '%T'", message)

	}

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't marshal: %v", err)
	}

	return data, nil
}

// Decode implements serde.FormatEngine. It populates the message from the JSON
// data if appropriate, otherwise it returns an error.
func (f msgFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := Message{}

	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize message: %v", err)
	}

	if m.Election != nil {
		return m.Election, nil
	}

	if m.Ballot != nil {
		return m.Ballot, nil
	}

	if m.Configuration != nil {
		return m.Configuration, nil
	}

	if m.Subject != nil {
		return m.Subject, nil
	}

	if m.Select != nil {
		return m.Select, nil
	}

	if m.Rank != nil {
		return m.Rank, nil
	}

	if m.Text != nil {
		return m.Text, nil
	}

	return nil, xerrors.New("message is empty")

}
