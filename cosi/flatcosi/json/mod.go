package json

import "encoding/json"

// Request is the JSON message sent to request signature.
type Request struct {
	Message json.RawMessage
}

// Response is the JSON message sent to respond with a signature.
type Response struct {
	Signature json.RawMessage
}
