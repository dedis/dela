package types

import (
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/registry"
	"golang.org/x/xerrors"
)

var requestFormats = registry.NewSimpleRegistry()

// RegisterRequestFormat registers the engine for the provided format.
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

// NewProposal creates a new proposal message.
func NewProposal(value serde.Message) Proposal {
	return Proposal{
		value: value,
	}
}

// GetValue returns the value of the proposal message.
func (p Proposal) GetValue() serde.Message {
	return p.value
}

// Serialize implements serde.Message.
func (p Proposal) Serialize(ctx serde.Context) ([]byte, error) {
	format := requestFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, p)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode proposal: %v", err)
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

// NewMessage creates a new message for the node and the value.
func NewMessage(node int64, value serde.Message) Message {
	return Message{
		node:  node,
		value: value,
	}
}

// GetNode returns the node index.
func (m Message) GetNode() int64 {
	return m.node
}

// GetValue returns the value wrapped by the message.
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

// NewMessageSet creates a new set of messages.
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

// GetMessages returns the list of messages.
func (mset MessageSet) GetMessages() []Message {
	messages := make([]Message, 0, len(mset.messages))
	for _, msg := range mset.messages {
		messages = append(messages, msg)
	}

	return messages
}

// GetTimeStep returns the time step.
func (mset MessageSet) GetTimeStep() uint64 {
	return mset.timeStep
}

// GetNode returns the node index.
func (mset MessageSet) GetNode() int64 {
	return mset.node
}

// Reduce returns a new message set with the messages of the provided nodes.
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

// Merge merges the two message sets.
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
		return nil, xerrors.Errorf("couldn't encode message set: %v", err)
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

// NewRequestMessageSet creates a new request message set message.
func NewRequestMessageSet(step uint64, nodes []int64) RequestMessageSet {
	return RequestMessageSet{
		timeStep: step,
		nodes:    nodes,
	}
}

// GetTimeStep returns the time step.
func (req RequestMessageSet) GetTimeStep() uint64 {
	return req.timeStep
}

// GetNodes returns the list of node indices.
func (req RequestMessageSet) GetNodes() []int64 {
	return append([]int64{}, req.nodes...)
}

// Serialize implements serde.Message.
func (req RequestMessageSet) Serialize(ctx serde.Context) ([]byte, error) {
	format := requestFormats.Get(ctx.GetFormat())

	data, err := format.Encode(ctx, req)
	if err != nil {
		return nil, xerrors.Errorf("couldn't encode request: %v", err)
	}

	return data, nil
}

// MsgKeyFac is the key of the message factory.
type MsgKeyFac struct{}

// RequestFactory is a message factory.
//
// - implements serde.Factory
type RequestFactory struct {
	mFactory serde.Factory
}

// NewRequestFactory creates a new request message factory.
func NewRequestFactory(f serde.Factory) RequestFactory {
	return RequestFactory{
		mFactory: f,
	}
}

// Deserialize implements serde.Factory.
func (f RequestFactory) Deserialize(ctx serde.Context, data []byte) (serde.Message, error) {
	format := requestFormats.Get(ctx.GetFormat())

	ctx = serde.WithFactory(ctx, MsgKeyFac{}, f.mFactory)

	msg, err := format.Decode(ctx, data)
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode request: %v", err)
	}

	return msg, nil
}
