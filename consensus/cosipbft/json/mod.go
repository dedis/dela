package json

import "encoding/json"

type ForwardLink struct {
	From      []byte
	To        []byte
	Prepare   json.RawMessage
	Commit    json.RawMessage
	ChangeSet json.RawMessage
}

type Chain []ForwardLink

type PrepareRequest struct {
	Message   json.RawMessage
	Signature json.RawMessage
	Chain     json.RawMessage
}

type CommitRequest struct {
	To      []byte
	Prepare json.RawMessage
}

type Request struct {
	Prepare *PrepareRequest
	Commit  *CommitRequest
}

type PropagateRequest struct {
	To     []byte
	Commit json.RawMessage
}
