// Package serde defines the primitives to serialize and deserialize (serde)
// network messages.
//
// The format can be chosen among three options:
// - JSON
// - Gob
// - Protobuf
package serde

import "io"

// Message is the interface a data model should implemented to be serialized and
// deserialized.
type Message interface {
	// VisitJSON should return a JSON data structure to encode.
	VisitJSON(Serializer) (interface{}, error)

	// VisitGob should return a gob data structure to encode.
	VisitGob(Serializer) (interface{}, error)

	// VisitProto should return a protobuf message to encode.
	VisitProto(Serializer) (interface{}, error)
}

// FactoryInput is the input provided to the factory when visiting. The input
// can be fed to an compatible interface to deserialize it.
type FactoryInput interface {
	// GetSerializer returns the serializer of the context.
	GetSerializer() Serializer

	// Feed writes the input into the given interface.
	Feed(interface{}) error
}

// Factory is the interface to implement to instantiate a data model from the
// raw message.
type Factory interface {
	// VisitJSON should return the message implementation of the JSON-encoded
	// input.
	VisitJSON(FactoryInput) (Message, error)

	// VisitGob should return the message implementation of the gob-encoded
	// input.
	VisitGob(FactoryInput) (Message, error)

	// VisitProto should return the message implementation of the
	// protobuf-encoded input.
	VisitProto(FactoryInput) (Message, error)
}

// Serializer is an interface that provides promitives to serialize and
// deserialize a data model.
type Serializer interface {
	// Serialize takes a message and returns the byte slice after serialization.
	Serialize(Message) ([]byte, error)

	// Deserialization takes the byte slice and the factory to instantiate the
	// message implementation.
	Deserialize([]byte, Factory, interface{}) error
}

type Fingerprinter interface {
	Fingerprint(io.Writer) error
}
