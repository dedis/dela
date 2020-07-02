package json

import "encoding/json"

// Record is a JSON record
type Record struct {
	K  []byte
	C  []byte
	AC json.RawMessage
}
