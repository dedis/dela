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

type PropagateGenesis struct {
	Genesis json.RawMessage
}

type BlockRequest struct {
	From uint64
	To   uint64
}

type BlockResponse struct {
	Block json.RawMessage
}
