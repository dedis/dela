package json

import "encoding/json"

type GenesisPayload struct {
	Roster json.RawMessage
	Root   []byte
}

type Transactions []json.RawMessage

type BlockPayload struct {
	Transactions Transactions
	Root         []byte
}

type Blueprint struct {
	Transactions Transactions
}

type Message struct {
	Blueprint      *Blueprint
	GenesisPayload *GenesisPayload
	BlockPayload   *BlockPayload
}
