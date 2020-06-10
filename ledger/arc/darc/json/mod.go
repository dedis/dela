package json

import "encoding/json"

// Access is the JSON message for a distributed access control.
type Access struct {
	Rules map[string][]string
}

// ClientTask is a JSON message for a DARC transaction task.
type ClientTask struct {
	Key    []byte
	Access json.RawMessage
}
