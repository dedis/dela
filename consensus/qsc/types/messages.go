package types

import (
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
)

var requestFormats = registry.NewSimpleRegistry()

func RegisterRequestFormat(c serde.Format, f serde.FormatEngine) {
	requestFormats.Register(c, f)
}

// Proposal is a message to wrap a proposal so that it can be deserialized
// alongside other request messages.
//
// - implements serde.Message
type Proposal struct {
	value serde.Message
}

func NewProposal(value serde.Message) Proposal {
	return Proposal{
		value: value,
	}
}

func (p Proposal) GetValue() serde.Message {
	return p.value
}

// Serialize implements serde.Message.
func (p Proposal) Serialize(ctx serde.Context) ([]byte, error) {
	format := requestFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, p)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// Message is a message sent by a node for the round.
//
// - implements serde.Message
type Message struct {
	node  int64
	value serde.Message
}

func NewMessage(node int64, value serde.Message) Message {
	return Message{
		node:  node,
		value: value,
	}
}

func (m Message) GetNode() int64 {
	return m.node
}

func (m Message) GetValue() serde.Message {
	return m.value
}

// MessageSet is a set of messages mapped by node index.
//
// - implements serde.Message
type MessageSet struct {
	messages map[int64]Message
	timeStep uint64
	node     int64
}

func NewMessageSet(node int64, step uint64, msgs ...Message) MessageSet {
	messages := make(map[int64]Message)
	for _, msg := range msgs {
		messages[msg.node] = msg
	}

	return MessageSet{
		node:     node,
		timeStep: step,
		messages: messages,
	}
}

func (mset MessageSet) GetMessages() []Message {
	messages := make([]Message, 0, len(mset.messages))
	for _, msg := range mset.messages {
		messages = append(messages, msg)
	}

	return messages
}

func (mset MessageSet) GetTimeStep() uint64 {
	return mset.timeStep
}

func (mset MessageSet) GetNode() int64 {
	return mset.node
}

func (mset MessageSet) Reduce(nodes []int64) MessageSet {
	messages := make(map[int64]Message)
	for _, key := range nodes {
		messages[key] = mset.messages[key]
	}

	return MessageSet{
		node:     mset.node,
		timeStep: mset.timeStep,
		messages: messages,
	}
}

func (mset MessageSet) Merge(other MessageSet) MessageSet {
	messages := make(map[int64]Message)

	for key, value := range mset.messages {
		messages[key] = value
	}
	for key, value := range other.messages {
		messages[key] = value
	}

	return MessageSet{
		node:     mset.node,
		timeStep: mset.timeStep,
		messages: messages,
	}
}

// Serialize implements serde.Message.
func (mset MessageSet) Serialize(ctx serde.Context) ([]byte, error) {
	format := requestFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, mset)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// RequestMessageSet is a request message for missing messages.
//
// - implements serde.Message
type RequestMessageSet struct {
	timeStep uint64
	nodes    []int64
}

func NewRequestMessageSet(step uint64, nodes []int64) RequestMessageSet {
	return RequestMessageSet{
		timeStep: step,
		nodes:    nodes,
	}
}

func (req RequestMessageSet) GetTimeStep() uint64 {
	return req.timeStep
}

func (req RequestMessageSet) GetNodes() []int64 {
	return append([]int64{}, req.nodes...)
}

// Serialize implements serde.Message.
func (req RequestMessageSet) Serialize(ctx serde.Context) ([]byte, error) {
	format := requestFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, req)
	if err != nil {
		return nil, err
	}

	return data, nil
}

type MsgKey struct{}

// RequestFactory is a message factory.
//
// - implements serde.Factory
type RequestFactory struct {
	mFactory serde.Factory
}

func NewRequestFactory(f serde.Factory) RequestFactory {
	return RequestFactory{
		mFactory: f,
	}
}

// Deserialize implements serde.Factory.
func (f RequestFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := requestFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, MsgKey{}, f.mFactory)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, err
	}

	return msg, nil
}
