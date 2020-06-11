package json

import "encoding/json"

// Player is a JSON message that contains the address and the public key of a
// new participant.
type Player struct {
	Address   []byte
	PublicKey json.RawMessage
}

// ChangeSet is a JSON message of the change set of an authority.
type ChangeSet struct {
	Remove []uint32
	Add    []Player
}

// Address is a JSON message for an address.
type Address []byte

// Roster is a JSON message for a roster.
type Roster []Player
