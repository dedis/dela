package qsc

import (
	"go.dedis.ch/dela/consensus/qsc/json"
	"go.dedis.ch/dela/serde"
	"golang.org/x/xerrors"
)

// Proposal is a message to wrap a proposal so that it can be deserialized
// alongside other request messages.
//
// - implements serde.Message
type Proposal struct {
	serde.UnimplementedMessage

	value serde.Message
}

// VisitJSON implements serde.Message. It serializes the proposal message in
// JSON format.
func (p Proposal) VisitJSON(ser serde.Serializer) (interface{}, error) {
	value, err := ser.Serialize(p.value)
	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize value: %v", err)
	}

	m := json.Proposal{
		Value: value,
	}

	return json.Request{Proposal: &m}, nil
}

// Message is a message sent by a node for the round.
//
// - implements serde.Message
type Message struct {
	serde.UnimplementedMessage

	node  int64
	value serde.Message
}

// VisitJSON implements serde.Message.
func (m Message) VisitJSON(ser serde.Serializer) (interface{}, error) {
	var data []byte
	var err error

	switch value := m.value.(type) {
	case MessageSet:
		data, err = ser.Serialize(value)
	default:
		data, err = ser.Serialize(Proposal{value: value})
	}

	if err != nil {
		return nil, xerrors.Errorf("couldn't serialize value: %v", err)
	}

	msg := json.Message{
		Node:  m.node,
		Value: data,
	}

	return msg, nil
}

// MessageSet is a set of messages mapped by node index.
//
// - implements serde.Message
type MessageSet struct {
	serde.UnimplementedMessage

	messages map[int64]Message
	timeStep uint64
	node     int64
}

// VisitJSON implements serde.Message.
func (mset MessageSet) VisitJSON(ser serde.Serializer) (interface{}, error) {
	messages := make(map[int64]json.Message)
	for index, msg := range mset.messages {
		raw, err := msg.VisitJSON(ser)
		if err != nil {
			return nil, xerrors.Errorf("couldn't serialize message: %v", err)
		}

		messages[index] = raw.(json.Message)
	}

	m := json.MessageSet{
		Node:     mset.node,
		TimeStep: mset.timeStep,
		Messages: messages,
	}

	return json.Request{MessageSet: &m}, nil
}

// RequestMessageSet is a request message for missing messages.
//
// - implements serde.Message
type RequestMessageSet struct {
	serde.UnimplementedMessage

	timeStep uint64
	nodes    []int64
}

// VisitJSON implements serde.Message.
func (req RequestMessageSet) VisitJSON(serde.Serializer) (interface{}, error) {
	m := json.RequestMessageSet{
		TimeStep: req.timeStep,
		Nodes:    req.nodes,
	}

	return json.Request{RequestMessageSet: &m}, nil
}

// RequestFactory is a message factory.
//
// - implements serde.Factory
type RequestFactory struct {
	serde.UnimplementedFactory

	mFactory serde.Factory
}

// VisitJSON implements serde.Factory.
func (f RequestFactory) VisitJSON(in serde.FactoryInput) (serde.Message, error) {
	m := json.Request{}
	err := in.Feed(&m)
	if err != nil {
		return nil, xerrors.Errorf("couldn't deserialize request: %v", err)
	}

	if m.MessageSet != nil {
		messages := make(map[int64]Message)
		for index, msg := range m.MessageSet.Messages {
			var value serde.Message
			err = in.GetSerializer().Deserialize(msg.Value, f, &value)
			if err != nil {
				return nil, xerrors.Errorf("couldn't deserialize value: %v", err)
			}

			messages[index] = Message{
				node:  msg.Node,
				value: value,
			}
		}

		mset := MessageSet{
			node:     m.MessageSet.Node,
			timeStep: m.MessageSet.TimeStep,
			messages: messages,
		}

		return mset, nil
	}

	if m.RequestMessageSet != nil {
		req := RequestMessageSet{
			timeStep: m.RequestMessageSet.TimeStep,
			nodes:    m.RequestMessageSet.Nodes,
		}

		return req, nil
	}

	if m.Proposal != nil {
		var value serde.Message
		err = in.GetSerializer().Deserialize(m.Proposal.Value, f.mFactory, &value)
		if err != nil {
			return nil, xerrors.Errorf("couldn't deserialize value: %v", err)
		}

		return value, nil
	}

	return nil, xerrors.New("message is empty")
}
