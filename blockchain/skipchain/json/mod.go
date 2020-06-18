package json

import "encoding/json"

// Blueprint is a JSON message to send a proposal.
type Blueprint struct {
	Index    uint64
	Previous []byte
	Payload  []byte
}

// SkipBlock is the JSON message for a block.
type SkipBlock struct {
	Index     uint64
	GenesisID []byte
	Backlink  []byte
	Payload   json.RawMessage
}

// VerifiableBlock is the JSON message for a verifiable block.
type VerifiableBlock struct {
	Block json.RawMessage
	Chain json.RawMessage
}

// PropagateGenesis the the JSON message to share a genesis block.
type PropagateGenesis struct {
	Genesis json.RawMessage
}

// BlockRequest is the JSON message to request a chain of blocks.
type BlockRequest struct {
	From uint64
	To   uint64
}

// BlockResponse is the response of a block request.
type BlockResponse struct {
	Block json.RawMessage
}

type Message struct {
	Propagate *PropagateGenesis `json:",omitempty"`
	Request   *BlockRequest     `json:",omitempty"`
	Response  *BlockResponse    `json:",omitempty"`
}
