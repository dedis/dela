package json

import "encoding/json"

type Access struct {
	Rules map[string][]string
}

type ClientTask struct {
	Key    []byte
	Access json.RawMessage
}
