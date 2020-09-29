// Package serde defines the serialization and deserialization mechanisms.
//
// Example of a JSON serialization and deserialization:
//
// 	ctx := json.NewContext()
// 	data, err := msg.Serialize(ctx)
// 	checkError(err)
//
// 	msg, err := factory.Deserialize(ctx, data)
// 	checkError(err)
//
package serde

import "io"

// Format is the identifier type of a format implementation.
type Format string

const (
	// FormatJSON is the identifier for JSON formats.
	FormatJSON Format = "JSON"
)

// Message is the interface that a message must implement.
type Message interface {
	// Serialize serializes the object by complying to the context format.
	Serialize(ctx Context) ([]byte, error)
}

// Factory is the interface that a message factory must implement.
type Factory interface {
	// Deserialize deserializes the message instantiated from the data.
	Deserialize(ctx Context, data []byte) (Message, error)
}

// FormatEngine is the interface that a format implementation must implement.
type FormatEngine interface {
	// Encode marshals the message according to the format definition.
	Encode(ctx Context, message Message) ([]byte, error)

	// Decode unmarshal a message according to the format definition.
	Decode(ctx Context, data []byte) (Message, error)
}

// Fingerprinter is an interface to fingerprint an object.
type Fingerprinter interface {
	// Fingerprint writes a deterministic binary representation of the object
	// into the writer.
	Fingerprint(writer io.Writer) error
}
