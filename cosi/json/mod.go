package json

import "encoding/json"

// Request is the JSON message sent to request signature.
type Request struct {
	Value json.RawMessage
}

// Response is the JSON message sent to respond with a signature.
type Response struct {
	Signature json.RawMessage
}

// Message is a JSON container to differentiate the different messages of flat
// collective signing.
type Message struct {
	Request  *Request  `json:",omitempty"`
	Response *Response `json:",omitempty"`
}
