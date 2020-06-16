package json

import "encoding/json"

type Signature struct {
	Mask      []byte
	Aggregate json.RawMessage
}
