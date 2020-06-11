package json

import "encoding/json"

// ForwardLink is the JSON message for a forward link.
type ForwardLink struct {
	From      []byte
	To        []byte
	Prepare   json.RawMessage
	Commit    json.RawMessage
	ChangeSet json.RawMessage
}

// Chain is the JSON message for the forward link chain.
type Chain []ForwardLink

// PrepareRequest is the JSON message to send the prepare request for a new
// proposal.
type PrepareRequest struct {
	Message   json.RawMessage
	Signature json.RawMessage
	Chain     json.RawMessage
}

// CommitRequest is the JSON message that contains the prepare signature to
// commit to a new proposal.
type CommitRequest struct {
	To      []byte
	Prepare json.RawMessage
}

// Request is a wrapper of the request JSON messages. It allows both types to be
// deserialized by the same factory.
type Request struct {
	Prepare *PrepareRequest
	Commit  *CommitRequest
}

// PropagateRequest is the JSON message sent when a proposal has been accepted
// by the network.
type PropagateRequest struct {
	To     []byte
	Commit json.RawMessage
}
