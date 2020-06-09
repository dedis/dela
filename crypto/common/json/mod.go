package json

// Algorithm is a common JSON message to identify which algorithm is used in a
// message.
type Algorithm struct {
	Name string
}

// PublicKey is the common JSON message for a public key. It contains the
// algorithm and the data to deserialize.
type PublicKey struct {
	Algorithm
	Data []byte
}

// Signature is the common JSON message for a signature. It contains the
// algorithm and the data to deserialize.
type Signature struct {
	Algorithm
	Data []byte
}
