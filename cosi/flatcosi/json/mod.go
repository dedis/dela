package json

import "encoding/json"

type Request struct {
	Message json.RawMessage
}

type Response struct {
	Signature json.RawMessage
}
