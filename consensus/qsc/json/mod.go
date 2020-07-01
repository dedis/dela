package json

import (
	"encoding/json"

	"go.dedis.ch/dela/consensus/qsc/types"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

func init() {
	types.RegisterHistoryFormat(serde.CodecJSON, historyFormat{})
	types.RegisterRequestFormat(serde.CodecJSON, requestFormat{})
}

type Epoch struct {
	Hash   []byte
	Random int64
}

type History []Epoch

// Message is the JSON message for messages sent by nodes for a round.
type Message struct {
	Node  int64
	Value json.RawMessage
}

type Proposal struct {
	Value json.RawMessage
}

// MessageSet is the JSON message for a set of messages.
type MessageSet struct {
	Node     int64
	TimeStep uint64
	Messages map[int64]Message
}

type RequestMessageSet struct {
	TimeStep uint64
	Nodes    []int64
}

// Request is a container for JSON messages.
type Request struct {
	MessageSet        *MessageSet
	RequestMessageSet *RequestMessageSet
	Proposal          *Proposal
}

type historyFormat struct{}

func (f historyFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	hist, ok := msg.(types.History)
	if !ok {
		return nil, xerrors.Errorf("invalid history of type '%T'", msg)
	}

	epochs := make([]Epoch, len(hist.GetEpochs()))
	for i, epoch := range hist.GetEpochs() {
		epochs[i] = Epoch{
			Hash:   epoch.GetHash(),
			Random: epoch.GetRandom(),
		}
	}

	m := History(epochs)

	data, err := ctx.Marshal(m)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (f historyFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := History{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize message: %v", err)
	}

	epochs := make([]types.Epoch, len(m))
	for i, e := range m {
		epochs[i] = types.NewEpoch(e.Hash, e.Random)
	}

	return types.NewHistory(epochs...), nil
}

type requestFormat struct{}

func (f requestFormat) Encode(ctx serde.Context, msg serde.Message) ([]byte, error) {
	var req Request

	switch in := msg.(type) {
	case types.Proposal:
		value, err := in.GetValue().Serialize(ctx)
		if err != nil {
			return nil, xerrors.Errorf("couldn't serialize value: %v", err)
		}

		m := Proposal{
			Value: value,
		}

		req = Request{Proposal: &m}
	case types.MessageSet:
		messages := make(map[int64]Message)
		for _, msg := range in.GetMessages() {
			m, err := f.encodeMessage(ctx, msg)
			if err != nil {
				return nil, xerrors.Errorf("couldn't serialize message: %v", err)
			}

			messages[msg.GetNode()] = m
		}

		m := MessageSet{
			Node:     in.GetNode(),
			TimeStep: in.GetTimeStep(),
			Messages: messages,
		}

		req = Request{MessageSet: &m}
	case types.RequestMessageSet:
		m := RequestMessageSet{
			TimeStep: in.GetTimeStep(),
			Nodes:    in.GetNodes(),
		}

		req = Request{RequestMessageSet: &m}
	default:
		return nil, xerrors.New("message is empty")
	}

	data, err := ctx.Marshal(req)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (f requestFormat) Decode(ctx serde.Context, data []byte) (serde.Message, error) {
	m := Request{}
	err := ctx.Unmarshal(data, &m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize request: %v", err)
	}

	if m.MessageSet != nil {
		messages := make([]types.Message, 0, len(m.MessageSet.Messages))

		for _, msg := range m.MessageSet.Messages {
			value, err := f.Decode(ctx, msg.Value)
			if err != nil {
				return nil, xerrors.Errorf("couldn't deserialize value: %v", err)
			}

			messages = append(messages, types.NewMessage(msg.Node, value))
		}

		mset := types.NewMessageSet(m.MessageSet.Node, m.MessageSet.TimeStep, messages...)

		return mset, nil
	}

	if m.RequestMessageSet != nil {
		req := types.NewRequestMessageSet(m.RequestMessageSet.TimeStep, m.RequestMessageSet.Nodes)

		return req, nil
	}

	if m.Proposal != nil {
		factory := ctx.GetFactory(types.MsgKey{})

		value, err := factory.Deserialize(ctx, m.Proposal.Value)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize value: %v", err)
		}

		return value, nil
	}

	return nil, xerrors.New("message is empty")
}

func (f requestFormat) encodeMessage(ctx serde.Context, msg types.Message) (Message, error) {
	var data []byte
	var err error

	switch value := msg.GetValue().(type) {
	case types.MessageSet:
		data, err = f.Encode(ctx, value)
	default:
		data, err = f.Encode(ctx, types.NewProposal(value))
	}

	if err != nil {
		return Message{}, err
	}

	m := Message{
		Node:  msg.GetNode(),
		Value: data,
	}

	return m, nil
}
