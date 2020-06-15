package json

import "encoding/json"

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
