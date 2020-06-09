// Package json defines the JSON messages for the basic transactions.
package json

import "encoding/json"

// Task is a container of a task with its type and the raw value to deserialize.
type Task struct {
	Type  string
	Value json.RawMessage
}

// Transaction is a combination of a given task and some metadata including the
// nonce to prevent replay attack and the signature to prove the identity of the
// client.
type Transaction struct {
	Nonce     uint64
	Identity  json.RawMessage
	Signature json.RawMessage
	Task      Task
}
