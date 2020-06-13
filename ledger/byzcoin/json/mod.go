package json

import "encoding/json"

// GenesisPayload is the JSON message of a genesis payload.
type GenesisPayload struct {
	Roster json.RawMessage
	Root   []byte
}

// Transactions is the JSON message for a list of transactions.
type Transactions []json.RawMessage

// BlockPayload is the JSON message for a block payload.
type BlockPayload struct {
	Transactions Transactions
	Root         []byte
}

// Blueprint is the JSON message for a new block proposal.
type Blueprint struct {
	Transactions Transactions
}

// Message is a JSON message container to deserialize any of the types.
type Message struct {
	Blueprint      *Blueprint
	GenesisPayload *GenesisPayload
	BlockPayload   *BlockPayload
}
