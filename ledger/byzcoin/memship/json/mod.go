package json

import "encoding/json"

// Task is the JSON message for the client task.
type Task struct {
	Remove    []uint32
	Address   []byte
	PublicKey json.RawMessage
}
