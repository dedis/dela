package json

// Blueprint is a JSON message to send a proposal.
type Blueprint struct {
	Index    uint64
	Previous []byte
	Payload  []byte
}
